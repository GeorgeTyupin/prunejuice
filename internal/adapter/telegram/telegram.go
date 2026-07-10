// Package telegram adapts the go-telegram-bot-api client to the
// domain.Notifier port. It is only wired in when the operator supplies a bot
// token and chat id, so the core library never imports a network client it
// does not use.
package telegram

import (
	"context"
	"errors"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/GeorgeTyupin/prunejuice/internal/domain"
)

// Notifier delivers alerts to a Telegram chat.
type Notifier struct {
	bot    *tgbotapi.BotAPI
	chatID int64
}

// New builds a Telegram notifier. It returns an error (rather than panicking)
// when credentials are missing or the token is rejected, so the CLI can fail
// fast with a clear message. NewBotAPI performs a getMe call, which validates
// the token against Telegram.
func New(token string, chatID int64) (*Notifier, error) {
	if token == "" {
		return nil, errors.New("telegram: bot token is empty")
	}
	if chatID == 0 {
		return nil, errors.New("telegram: chat id is empty")
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram: initialise bot: %w", err)
	}
	return &Notifier{bot: bot, chatID: chatID}, nil
}

// Notify implements domain.Notifier.
func (n *Notifier) Notify(_ context.Context, alert domain.Alert) error {
	msg := tgbotapi.NewMessage(n.chatID, alert.Text())
	msg.DisableWebPagePreview = true
	if _, err := n.bot.Send(msg); err != nil {
		return fmt.Errorf("telegram: send message: %w", err)
	}
	return nil
}
