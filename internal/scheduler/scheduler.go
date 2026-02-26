// internal/scheduler/scheduler.go
package scheduler

import (
	"log/slog"

	"github.com/robfig/cron/v3"
	"github.com/user/gopherclaw/internal/state"
)

// Handler is the callback invoked when a scheduled task fires.
type Handler func(sessionKey, prompt string)

// Scheduler evaluates cron expressions from the task store and fires tasks
// through a handler callback.
type Scheduler struct {
	store   *state.TaskStore
	handler Handler
	cron    *cron.Cron
}

// cronParser accepts both standard 5-field cron expressions and 6-field
// expressions with an optional seconds field.
var cronParser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// New creates a new Scheduler backed by the given task store. The handler is
// called each time a scheduled task fires.
func New(store *state.TaskStore, handler Handler) *Scheduler {
	return &Scheduler{
		store:   store,
		handler: handler,
		cron:    cron.New(cron.WithParser(cronParser)),
	}
}

// Start loads tasks from the store, registers enabled tasks that have a
// schedule as cron entries, and starts the cron ticker.
func (s *Scheduler) Start() error {
	tasks, err := s.store.List()
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if task.Schedule == "" || !task.Enabled {
			continue
		}

		// Capture loop variables for the closure.
		sessionKey := task.SessionKey
		prompt := task.Prompt
		schedule := task.Schedule
		name := task.Name

		_, err := s.cron.AddFunc(schedule, func() {
			slog.Info("cron firing task", "name", name, "session_key", sessionKey)
			s.handler(sessionKey, prompt)
		})
		if err != nil {
			slog.Error("invalid cron schedule", "name", name, "schedule", schedule, "error", err)
			continue
		}
		slog.Info("scheduled task", "name", name, "schedule", schedule)
	}

	s.cron.Start()
	return nil
}

// Reload stops the existing cron, creates a new one, and calls Start() again.
func (s *Scheduler) Reload() error {
	s.cron.Stop()
	s.cron = cron.New(cron.WithParser(cronParser))
	return s.Start()
}

// Stop stops the cron ticker.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}
