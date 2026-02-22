package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/types"
)

const maxTelegramMessage = 4096

// Adapter bridges Telegram to the gateway.
type Adapter struct {
	bot      *tgbotapi.BotAPI
	gateway  *gateway.Gateway
	events   types.EventStore
	sessions types.SessionStore
}

// New creates a Telegram adapter.
func New(token string, gw *gateway.Gateway, events types.EventStore, sessions types.SessionStore) (*Adapter, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}
	return &Adapter{
		bot:      bot,
		gateway:  gw,
		events:   events,
		sessions: sessions,
	}, nil
}

// Start begins long-polling for Telegram updates.
func (a *Adapter) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := a.bot.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			if update.Message == nil || update.Message.Text == "" {
				continue
			}
			a.handleMessage(ctx, update.Message)
		case <-ctx.Done():
			a.bot.StopReceivingUpdates()
			return
		}
	}
}

func (a *Adapter) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	// Handle commands
	if msg.IsCommand() {
		a.handleCommand(ctx, msg)
		return
	}

	chatID := msg.Chat.ID
	event := &types.InboundEvent{
		Source:     "telegram",
		SessionKey: buildSessionKey(msg.From.ID, msg.Chat.ID),
		UserID:     strconv.FormatInt(msg.From.ID, 10),
		Text:       msg.Text,
	}

	err := a.gateway.HandleInbound(ctx, event, gateway.WithOnComplete(func(response string) {
		a.sendResponse(chatID, response)
	}))
	if err != nil {
		log.Printf("handle inbound error: %v", err)
		a.sendResponse(chatID, "Sorry, I encountered an error processing your message.")
	}
}

func (a *Adapter) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	switch msg.Command() {
	case "start":
		a.sendResponse(chatID, "Hello! I'm Gopherclaw, your AI assistant. Send me a message to get started.")

	case "new":
		a.sendResponse(chatID, "Starting a new session. Previous conversation has been archived.")

	case "status":
		key := buildSessionKey(msg.From.ID, msg.Chat.ID)
		sid, err := a.sessions.ResolveOrCreate(ctx, key, "default")
		if err != nil {
			a.sendResponse(chatID, "Error fetching status.")
			return
		}
		count, err := a.events.Count(ctx, sid)
		if err != nil {
			a.sendResponse(chatID, "Error fetching status.")
			return
		}
		a.sendResponse(chatID, fmt.Sprintf("Session: %s\nMessages: %d", sid, count))

	default:
		a.sendResponse(chatID, "Unknown command. Available: /start, /new, /status")
	}
}

func (a *Adapter) sendResponse(chatID int64, text string) {
	parts := splitMessage(text)
	for _, part := range parts {
		msg := tgbotapi.NewMessage(chatID, part)
		msg.ParseMode = "Markdown"
		if _, err := a.bot.Send(msg); err != nil {
			// Retry without markdown if it fails
			msg.ParseMode = ""
			if _, err := a.bot.Send(msg); err != nil {
				log.Printf("send message error: %v", err)
			}
		}
	}
}

func splitMessage(text string) []string {
	if len(text) <= maxTelegramMessage {
		return []string{text}
	}
	var parts []string
	for len(text) > 0 {
		end := maxTelegramMessage
		if end > len(text) {
			end = len(text)
		}
		parts = append(parts, text[:end])
		text = text[end:]
	}
	return parts
}

func buildSessionKey(userID, chatID int64) types.SessionKey {
	return types.NewSessionKey("telegram",
		strconv.FormatInt(userID, 10),
		strconv.FormatInt(chatID, 10),
	)
}
