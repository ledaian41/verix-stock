package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/ledaian41/verix-stock/internal/core/db"
	"github.com/ledaian41/verix-stock/internal/modules/article"
	"github.com/ledaian41/verix-stock/internal/modules/watchlist"

	gp "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	maxRequestBodyBytes = 1 << 20 // 1 MiB
	tokenTTL            = 24 * time.Hour
	telegramAuthMaxAge  = 86400 // 1 day in seconds
	chatIDMinLen        = 1
	chatIDMaxLen        = 64
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	startTime := time.Now()

	if err := godotenv.Load(); err != nil {
		logger.Warn("no .env file found, using OS environment vars")
	}

	dsn := os.Getenv("SUPABASE_DB_URL")
	if dsn == "" {
		logger.Error("SUPABASE_DB_URL is required")
		os.Exit(1)
	}

	botToken := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if botToken == "" {
		logger.Error("TELEGRAM_BOT_TOKEN is required")
		os.Exit(1)
	}

	botUsername := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_USERNAME"))
	if botUsername == "" {
		logger.Error("TELEGRAM_BOT_USERNAME is required")
		os.Exit(1)
	}

	// DB setup with retry
	var gormDB *gorm.DB
	var err error
	for i := 1; i <= 5; i++ {
		gormDB, err = gorm.Open(gp.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		logger.Warn("failed to connect to database, retrying...", "attempt", i, "error", err)
		if i == 5 {
			logger.Error("could not connect to database after 5 attempts", "error", err)
			os.Exit(1)
		}
		time.Sleep(2 * time.Second)
	}
	db.DB = gormDB

	if err := gormDB.AutoMigrate(&article.Article{}, &watchlist.StockConfig{}); err != nil {
		logger.Error("auto-migrate failed", "error", err)
		os.Exit(1)
	}

	articleRepo := article.NewRepository(gormDB)
	watchlistRepo := watchlist.NewRepository(gormDB)

	// ─── Gin setup ────────────────────────────────────────────────────────────
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
		c.Next()
	})

	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("request", "method", c.Request.Method, "path", c.Request.URL.Path,
			"status", c.Writer.Status(), "latency", time.Since(start))
	})

	// ─── Public routes ────────────────────────────────────────────────────────

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "UP",
			"time":   time.Now().Format(time.RFC3339),
			"uptime": time.Since(startTime).Round(time.Second).String(),
		})
	})

	r.GET("/auth/bot", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"username": botUsername})
	})

	r.POST("/auth/telegram", telegramAuthHandler(botToken, logger))

	api := r.Group("/api")
	article.RegisterRoutes(api, articleRepo)

	r.StaticFile("/", "./web/index.html")

	// ─── Protected routes (Bearer token) ─────────────────────────────────────
	auth := authMiddleware(botToken, logger)

	config := r.Group("/config", auth)
	{
		config.POST("", upsertWatchlist(watchlistRepo, logger))
		config.GET("/:chat_id", getWatchlist(watchlistRepo, logger))
		config.DELETE("/:chat_id", deleteWatchlist(watchlistRepo, logger))
	}

	// ─── HTTP server ─────────────────────────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Info("api server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server fatal error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}
	logger.Info("server stopped cleanly")
}

// ── Auth helpers ──────────────────────────────────────────────────────────────

func sessionSecretKey(botToken string) []byte {
	h := sha256.Sum256([]byte(botToken))
	return h[:]
}

func verifyTelegramAuth(data map[string]string, botToken string) (bool, string) {
	receivedHash, ok := data["hash"]
	if !ok {
		return false, "<hash field missing>"
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+data[k])
	}
	dataCheckString := strings.Join(parts, "\n")
	secretKey := sessionSecretKey(botToken)
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(dataCheckString))
	return hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(receivedHash)), dataCheckString
}

func issueToken(chatID, botToken string) string {
	expiry := time.Now().Add(tokenTTL).Unix()
	payload := fmt.Sprintf("%s:%d", chatID, expiry)
	mac := hmac.New(sha256.New, sessionSecretKey(botToken))
	mac.Write([]byte(payload))
	return payload + ":" + hex.EncodeToString(mac.Sum(nil))
}

func verifyToken(token, botToken string) (string, error) {
	lastColon := strings.LastIndex(token, ":")
	if lastColon < 0 {
		return "", errors.New("malformed token")
	}
	sig := token[lastColon+1:]
	rest := token[:lastColon]
	secondColon := strings.LastIndex(rest, ":")
	if secondColon < 0 {
		return "", errors.New("malformed token")
	}
	expiryStr := rest[secondColon+1:]
	chatID := rest[:secondColon]
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return "", errors.New("token expired or invalid")
	}
	payload := chatID + ":" + expiryStr
	mac := hmac.New(sha256.New, sessionSecretKey(botToken))
	mac.Write([]byte(payload))
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(sig)) {
		return "", errors.New("invalid token signature")
	}
	return chatID, nil
}

func authMiddleware(botToken string, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			return
		}
		chatID, err := verifyToken(strings.TrimPrefix(authHeader, "Bearer "), botToken)
		if err != nil {
			logger.Warn("auth: token verification failed", "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("token_chat_id", chatID)
		c.Next()
	}
}

func validateChatID(chatID string) bool {
	n := len(chatID)
	if n < chatIDMinLen || n > chatIDMaxLen {
		return false
	}
	for i, r := range chatID {
		if i == 0 && r == '-' {
			continue
		}
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func telegramAuthHandler(botToken string, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		var rawPayload map[string]json.RawMessage
		if err := json.Unmarshal(body, &rawPayload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		data := make(map[string]string, len(rawPayload))
		for k, raw := range rawPayload {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				data[k] = s
			} else {
				data[k] = strings.TrimSpace(string(raw))
			}
		}
		authDate, err := strconv.ParseInt(data["auth_date"], 10, 64)
		if err != nil || time.Now().Unix()-authDate > telegramAuthMaxAge {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication data is outdated"})
			return
		}
		ok, dataCheckString := verifyTelegramAuth(data, botToken)
		if !ok {
			logger.Warn("telegramAuth: hash mismatch", "data_check_string", dataCheckString)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication verification failed"})
			return
		}
		chatID := data["id"]
		name := strings.TrimSpace(data["first_name"] + " " + data["last_name"])
		token := issueToken(chatID, botToken)
		c.JSON(http.StatusOK, gin.H{"token": token, "chat_id": chatID, "name": name})
	}
}

func ownerChatID(c *gin.Context) (int64, string, bool) {
	rawChatID := c.Param("chat_id")
	if rawChatID == "" {
		return 0, "", false
	}
	tokenChatID, _ := c.Get("token_chat_id")
	if tokenChatID.(string) != rawChatID {
		return 0, "", false
	}
	id, err := strconv.ParseInt(rawChatID, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return id, rawChatID, true
}

func upsertWatchlist(repo *watchlist.Repository, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input watchlist.StockConfig
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
			return
		}

		tokenChatID, _ := c.Get("token_chat_id")
		if tokenChatID.(string) != strconv.FormatInt(input.ChatID, 10) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}

		_, err := repo.Upsert(&input)
		if err != nil {
			logger.Error("upsertWatchlist", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save configuration"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "configuration synchronized successfuly"})
	}
}

func getWatchlist(repo *watchlist.Repository, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _, ok := ownerChatID(c)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		cfg, err := repo.GetByChatID(id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusOK, watchlist.StockConfig{ChatID: id, Symbols: []string{}})
				return
			}
			logger.Error("getWatchlist", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		c.JSON(http.StatusOK, cfg)
	}
}

func deleteWatchlist(repo *watchlist.Repository, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _, ok := ownerChatID(c)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		if err := repo.Delete(id); err != nil {
			logger.Error("deleteWatchlist", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}
