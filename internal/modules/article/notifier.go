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
)

type TelegramNotifier struct {
	token  string
	chatID string
}

func NewTelegramNotifier() *TelegramNotifier {
	return &TelegramNotifier{
		token:  os.Getenv("TELEGRAM_BOT_TOKEN"),
		chatID: os.Getenv("TELEGRAM_CHAT_ID"),
	}
}

func (n *TelegramNotifier) Notify(ctx context.Context, pub PublishedArticle) error {
	if n.token == "" || n.chatID == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not set")
	}

	// 1. Determine Icon based on Sentiment Score
	icon := "⚠️" // Neutral
	if pub.SentimentScore >= 0.3 {
		icon = "✅" // Positive
	} else if pub.SentimentScore <= -0.3 {
		icon = "❌" // Negative
	}

	// 2. Format Message Template
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

	// 3. Send to Telegram API
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.token)
	payload := map[string]interface{}{
		"chat_id":    n.chatID,
		"text":       msg,
		"parse_mode": "HTML", // Switched to HTML for stability
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram api error: status %d", resp.StatusCode)
	}

	return nil
}
