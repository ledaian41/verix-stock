package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionChatIDKey = "tg_chat_id"

// TelegramAuth validates the Telegram Login Widget data hash and sets chat_id in context.
// Docs: https://core.telegram.org/widgets/login#checking-authorization
func TelegramAuth() gin.HandlerFunc {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")

	// Generate the secret key: SHA256 of the bot token
	h := sha256.New()
	h.Write([]byte(botToken))
	secretKey := h.Sum(nil)

	return func(c *gin.Context) {
		// Read Telegram auth params from query (redirect from widget)
		params := map[string]string{
			"id":         c.Query("id"),
			"first_name": c.Query("first_name"),
			"username":   c.Query("username"),
			"photo_url":  c.Query("photo_url"),
			"auth_date":  c.Query("auth_date"),
		}
		receivedHash := c.Query("hash")

		if receivedHash == "" || params["id"] == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing telegram auth data"})
			c.Abort()
			return
		}

		// Check auth_date not older than 24h
		authDate, err := strconv.ParseInt(params["auth_date"], 10, 64)
		if err != nil || time.Now().Unix()-authDate > 86400 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "auth data expired"})
			c.Abort()
			return
		}

		// Build data-check-string: sorted "key=value\n" lines, excluding hash and empty values
		var lines []string
		for k, v := range params {
			if v != "" {
				lines = append(lines, fmt.Sprintf("%s=%s", k, v))
			}
		}
		sort.Strings(lines)
		dataCheckString := strings.Join(lines, "\n")

		// Compute HMAC-SHA256
		mac := hmac.New(sha256.New, secretKey)
		mac.Write([]byte(dataCheckString))
		expectedHash := hex.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(expectedHash), []byte(receivedHash)) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid telegram hash"})
			c.Abort()
			return
		}

		chatID, _ := strconv.ParseInt(params["id"], 10, 64)
		c.Set("chat_id", chatID)
		c.Set("first_name", params["first_name"])
		c.Next()
	}
}

// RequireSession is a lightweight session-based guard for the Dashboard HTML routes.
// It reads chat_id stored in a cookie/session after successful Telegram login callback.
func RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		chatIDStr, err := c.Cookie(sessionChatIDKey)
		if err != nil || chatIDStr == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Set("chat_id", chatID)
		c.Next()
	}
}
