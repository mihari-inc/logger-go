package mihari

import (
	"context"
	"log/slog"
	"slices"
)

// SlogHandler implements slog.Handler, allowing mihari to be used as a backend
// for the Go standard library structured logging package.
//
// Usage:
//
//	client := mihari.New("token")
//	logger := slog.New(mihari.NewSlogHandler(client))
//	logger.Info("hello", "key", "value")
type SlogHandler struct {
	client *Client
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

// NewSlogHandler creates a new slog.Handler backed by a mihari Client.
// The handler respects the client's minimum level setting.
func NewSlogHandler(client *Client, opts ...SlogHandlerOption) *SlogHandler {
	h := &SlogHandler{
		client: client,
		level:  client.cfg.minLevel.toSlogLevel(),
	}
	for _, o := range opts {
		o(h)
	}
	return h
}

// SlogHandlerOption configures a SlogHandler.
type SlogHandlerOption func(*SlogHandler)

// WithSlogLevel overrides the minimum slog level for the handler.
func WithSlogLevel(l slog.Level) SlogHandlerOption {
	return func(h *SlogHandler) {
		h.level = l
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle processes a slog.Record by converting it to a LogEntry and enqueuing it.
func (h *SlogHandler) Handle(_ context.Context, r slog.Record) error {
	lvl := levelFromSlog(r.Level)
	meta := h.client.buildMeta(nil)

	// Merge pre-set attrs.
	for _, a := range h.attrs {
		addAttr(meta, h.groups, a)
	}

	// Merge record attrs.
	r.Attrs(func(a slog.Attr) bool {
		addAttr(meta, h.groups, a)
		return true
	})

	entry := LogEntry{
		Timestamp: r.Time,
		Level:     lvl.String(),
		Message:   r.Message,
		Meta:      meta,
	}
	h.client.transport.enqueue(entry)
	return nil
}

// WithAttrs returns a new SlogHandler with the given attributes pre-set.
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SlogHandler{
		client: h.client,
		level:  h.level,
		attrs:  append(slices.Clone(h.attrs), attrs...),
		groups: slices.Clone(h.groups),
	}
}

// WithGroup returns a new SlogHandler that qualifies subsequent attributes
// under the given group name.
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &SlogHandler{
		client: h.client,
		level:  h.level,
		attrs:  slices.Clone(h.attrs),
		groups: append(slices.Clone(h.groups), name),
	}
}

// addAttr inserts a slog.Attr into the meta map, handling groups by nesting.
func addAttr(meta map[string]any, groups []string, a slog.Attr) {
	a = a.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}

	target := meta
	for _, g := range groups {
		nested, ok := target[g].(map[string]any)
		if !ok {
			nested = make(map[string]any)
			target[g] = nested
		}
		target = nested
	}

	if a.Value.Kind() == slog.KindGroup {
		for _, ga := range a.Value.Group() {
			innerGroups := groups
			if a.Key != "" {
				innerGroups = append(innerGroups, a.Key)
			}
			addAttr(meta, innerGroups, ga)
		}
		return
	}

	target[a.Key] = a.Value.Any()
}
