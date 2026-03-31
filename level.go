package mihari

import (
	"fmt"
	"log/slog"
	"strings"
)

// Level represents log severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns the lowercase string representation of a Level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return fmt.Sprintf("level(%d)", int(l))
	}
}

// ParseLevel converts a string to a Level. It is case-insensitive.
// Returns LevelInfo and an error for unrecognised strings.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	case "fatal":
		return LevelFatal, nil
	default:
		return LevelInfo, fmt.Errorf("mihari: unknown level %q", s)
	}
}

// toSlogLevel maps a mihari Level to log/slog.Level.
func (l Level) toSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	case LevelFatal:
		// slog has no Fatal; map to Error+4 so it always passes level checks.
		return slog.LevelError + 4
	default:
		return slog.LevelInfo
	}
}

// levelFromSlog converts a slog.Level to a mihari Level.
func levelFromSlog(sl slog.Level) Level {
	switch {
	case sl >= slog.LevelError+4:
		return LevelFatal
	case sl >= slog.LevelError:
		return LevelError
	case sl >= slog.LevelWarn:
		return LevelWarn
	case sl >= slog.LevelInfo:
		return LevelInfo
	default:
		return LevelDebug
	}
}
