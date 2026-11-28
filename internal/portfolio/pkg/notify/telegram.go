package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const DefaultTelegramAPI = "https://api.telegram.org"

// SendTelegram sends a message to a Telegram chat using the Bot API.
func SendTelegram(botToken, chatID, message string) error {
	return SendTelegramWithURL(DefaultTelegramAPI, botToken, chatID, message)
}

// SendTelegramWithURL sends a message to a custom Telegram Bot API base URL (useful for testing).
func SendTelegramWithURL(baseURL, botToken, chatID, message string) error {
	if botToken == "" || chatID == "" {
		return fmt.Errorf("telegram bot token or chat ID not provided")
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", baseURL, botToken)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}
	return nil
}
