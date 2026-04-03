package article

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ledaian41/verix-stock/internal/modules/watchlist"
)

type TelegramNotifier struct {
	token     string
	backupID  string
	watchlist *watchlist.Repository
}

func NewTelegramNotifier(wl *watchlist.Repository) *TelegramNotifier {
	return &TelegramNotifier{
		token:     os.Getenv("TELEGRAM_BOT_TOKEN"),
		backupID:  os.Getenv("TELEGRAM_CHAT_ID"),
		watchlist: wl,
	}
}

func (n *TelegramNotifier) Notify(ctx context.Context, pub PublishedArticle) error {
	if n.token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN not set")
	}

	// 1. Get all recipients (Watchlist + Backup)
	recipients := make(map[string]bool)
	if n.backupID != "" {
		recipients[n.backupID] = true
	}

	if n.watchlist != nil {
		chatIDs, err := n.watchlist.GetChatIDsBySymbol(pub.Ticker)
		if err == nil {
			for _, id := range chatIDs {
				recipients[fmt.Sprintf("%d", id)] = true
			}
		}
	}

	if len(recipients) == 0 {
		return fmt.Errorf("no recipients found (watchlist empty and TELEGRAM_CHAT_ID not set)")
	}

	// 2. Determine Icon based on Sentiment Score
	icon := "⚠️" // Neutral
	if pub.SentimentScore >= 0.3 {
		icon = "✅" // Positive
	} else if pub.SentimentScore <= -0.3 {
		icon = "❌" // Negative
	}

	// 3. Format Message Template
	tickerHTML := fmt.Sprintf("<code>%s</code>", strings.ToUpper(pub.Ticker))
	header := fmt.Sprintf("🚀 <b>TIN CỔ PHIẾU</b> %s - <b>%s</b>", tickerHTML, strings.ToUpper(time.Now().Format("02/01/2006")))
	separator := "───────────────────"

	msg := fmt.Sprintf(
		"%s\n%s\n"+
			"<b>%s ĐIỂM TIN CHÍNH: (%d tin mới)</b>\n%s"+
			"<b>📌 Tóm tắt:</b>\n%s",
		header,
		separator,
		icon,
		pub.ArticleCount,
		html.EscapeString(pub.Summary),
		html.EscapeString(pub.Conclusion),
	)

	// 4. Send to all recipients
	var lastErr error
	for chatID := range recipients {
		url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.token)
		payload := map[string]interface{}{
			"chat_id":    chatID,
			"text":       msg,
			"parse_mode": "HTML",
		}

		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("telegram api error for %s: status %d", chatID, resp.StatusCode)
		}
	}

	return lastErr
}
