package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type TelegramTestJob struct {
	NameSuffix string
}

func (j *TelegramTestJob) Name() string {
	return "telegram_test_" + j.NameSuffix
}

func (j *TelegramTestJob) Run(ctx context.Context) error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if token == "" || chatID == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not set")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	msg := fmt.Sprintf("🚀 [Verix Worker] Test %s triggered successfully!", j.NameSuffix)

	payload := map[string]string{
		"chat_id": chatID,
		"text":    msg,
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
		return fmt.Errorf("telegram api error: %d", resp.StatusCode)
	}

	return nil
}
