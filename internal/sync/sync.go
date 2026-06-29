package sync

import (
	"context"
	"time"

	"github.com/dibou/mcp-obsidian/internal/state"
)

type Syncer interface {
	EnsureFresh(context.Context) error
	Pull(context.Context) error
	Push(context.Context) error
	Status(context.Context) (Status, error)
	Enabled() bool
}

type Status struct {
	Enabled       bool      `json:"enabled"`
	LastPull      time.Time `json:"last_pull,omitempty"`
	LastPush      time.Time `json:"last_push,omitempty"`
	AgeSeconds    int64     `json:"age_seconds,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	LastErrorTime time.Time `json:"last_error_time,omitempty"`
}

type noop struct{}

func NewNoop() Syncer {
	return noop{}
}

func (noop) EnsureFresh(context.Context) error { return nil }
func (noop) Pull(context.Context) error        { return nil }
func (noop) Push(context.Context) error        { return nil }
func (noop) Enabled() bool                     { return false }

func (noop) Status(context.Context) (Status, error) {
	return Status{Enabled: false}, nil
}

func FromState(enabled bool, st state.Status) Status {
	now := time.Now().UTC()
	last := st.LastPull
	if st.LastPush.After(last) {
		last = st.LastPush
	}
	var age int64
	if !last.IsZero() {
		age = int64(now.Sub(last).Seconds())
	}
	return Status{
		Enabled:       enabled,
		LastPull:      st.LastPull,
		LastPush:      st.LastPush,
		AgeSeconds:    age,
		LastError:     st.LastError,
		LastErrorTime: st.LastErrorTime,
	}
}
