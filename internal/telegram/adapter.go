package telegram

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/types"
)

const maxTelegramMessage = 4096

// Adapter bridges Telegram to the gateway.
type Adapter struct {
	bot       *tgbotapi.BotAPI
	gateway   *gateway.Gateway
	events    types.EventStore
	sessions  types.SessionStore
	engine     *ctxengine.Engine
	toolNames  []string
	memoryPath string
}

// New creates a Telegram adapter.
func New(token string, gw *gateway.Gateway, events types.EventStore, sessions types.SessionStore, engine *ctxengine.Engine, toolNames []string, memoryPath string) (*Adapter, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}
	return &Adapter{
		bot:        bot,
		gateway:    gw,
		events:     events,
		sessions:   sessions,
		engine:     engine,
		toolNames:  toolNames,
		memoryPath: memoryPath,
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
		key := buildSessionKey(msg.From.ID, msg.Chat.ID)
		oldSID, err := a.sessions.Rotate(ctx, key)
		if err != nil {
			a.sendResponse(chatID, "Error creating new session.")
			return
		}
		if oldSID == "" {
			a.sendResponse(chatID, "No existing session. Send a message to start one.")
		} else {
			a.sendResponse(chatID, "New session started. Previous conversation has been archived.")
		}

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

	case "context":
		key := buildSessionKey(msg.From.ID, msg.Chat.ID)
		sid, err := a.sessions.ResolveOrCreate(ctx, key, "default")
		if err != nil {
			a.sendResponse(chatID, "Error fetching session.")
			return
		}
		session, err := a.sessions.Get(ctx, sid)
		if err != nil {
			a.sendResponse(chatID, "Error fetching session.")
			return
		}
		events, err := a.events.Tail(ctx, sid, 100)
		if err != nil {
			a.sendResponse(chatID, "Error loading events.")
			return
		}
		summary := a.engine.Summarize(session, events, a.toolNames)
		text := fmt.Sprintf("```\nContext Budget:\n"+
			"  Max tokens:      %d\n"+
			"  Output reserve:  %d\n"+
			"  Input budget:    %d\n\n"+
			"System Prompt:     %d tokens\n"+
			"Event History:     %d / %d tokens (%d of %d events)\n"+
			"Remaining:         %d tokens\n"+
			"```\n\n*System Prompt:*\n```\n%s\n```",
			summary.MaxTokens,
			summary.Reserve,
			summary.InputBudget,
			summary.SystemPromptTokens,
			summary.EventTokensUsed, summary.EventBudget, summary.EventsIncluded, summary.EventsTotal,
			summary.BudgetRemaining,
			summary.SystemPromptText,
		)
		a.sendResponse(chatID, text)

	case "memories":
		data, err := os.ReadFile(a.memoryPath)
		if err != nil || strings.TrimSpace(string(data)) == "" {
			a.sendResponse(chatID, "No memories stored yet.")
			return
		}
		a.sendResponse(chatID, fmt.Sprintf("*Stored Memories:*\n```\n%s```", string(data)))

	default:
		a.sendResponse(chatID, "Unknown command. Available: /start, /new, /status, /context, /memories")
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
