package mihari

import (
	"time"
)

// LogEntry is a single log record sent to the API.
// Extra metadata fields are stored in the Meta map and serialised as
// top-level JSON keys alongside "dt", "level", and "message".
type LogEntry struct {
	Timestamp time.Time         `json:"dt"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Meta      map[string]any    `json:"-"` // flattened during marshal
}

// MarshalJSON implements json.Marshaler.
// It produces {"dt":...,"level":...,"message":...,...extra_metadata}.
func (e LogEntry) MarshalJSON() ([]byte, error) {
	// Build a map so extra metadata sits at the top level.
	m := make(map[string]any, 3+len(e.Meta))
	m["dt"] = e.Timestamp.UTC().Format(time.RFC3339Nano)
	m["level"] = e.Level
	m["message"] = e.Message
	for k, v := range e.Meta {
		// Reserved keys are not overwritten.
		if k == "dt" || k == "level" || k == "message" {
			continue
		}
		m[k] = v
	}
	return marshalJSON(m)
}

// newEntry creates a LogEntry with the current timestamp.
func newEntry(level Level, msg string, meta map[string]any) LogEntry {
	return LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   msg,
		Meta:      meta,
	}
}
