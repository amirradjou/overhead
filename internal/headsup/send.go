package headsup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// Sender delivers a heads-up.
type Sender interface {
	Send(ctx context.Context, text string) error
}

// LogSender only logs: the heads-up runs without a phone until a Telegram bot is set up.
type LogSender struct{}

// Send logs text.
func (LogSender) Send(_ context.Context, text string) error {
	slog.Info("heads-up", "text", text)
	return nil
}

// Telegram sends through a bot to one chat.
type Telegram struct {
	Token, ChatID string
	Client        *http.Client
	// Base is the Bot API's address, https://api.telegram.org unless a test says otherwise.
	Base string
}

// Send posts text to the chat. Errors never carry the bot token, which sits in the request's path.
func (t *Telegram) Send(ctx context.Context, text string) error {
	base := t.Base
	if base == "" {
		base = "https://api.telegram.org"
	}
	form := url.Values{"chat_id": {t.ChatID}, "text": {text}, "disable_web_page_preview": {"true"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+t.Token+"/sendMessage",
		strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("telegram: bad request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.Client.Do(req)
	if err != nil {
		if ue, ok := errors.AsType[*url.Error](err); ok {
			return fmt.Errorf("telegram: %w", ue.Err)
		}
		return errors.New("telegram: request failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	return nil
}
