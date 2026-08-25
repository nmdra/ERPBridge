// Package logger provides structured logging, context propagation, and RFC 5424 / MCP logging handlers.
package logger

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	logListeners []chan []byte
	listenersMu  sync.RWMutex
	logBuffer    [][]byte
	bufferSize   = 1000
)

// Subscribe adds a new log channel subscriber to receive raw JSON log lines.
func Subscribe() chan []byte {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	ch := make(chan []byte, 100)
	logListeners = append(logListeners, ch)
	return ch
}

// Unsubscribe removes and closes a registered log channel subscriber.
func Unsubscribe(ch chan []byte) {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	for i, l := range logListeners {
		if l == ch {
			logListeners = append(logListeners[:i], logListeners[i+1:]...)
			close(ch)
			break
		}
	}
}

// GetRecentLogs returns a snapshot copy of recently buffered log messages.
func GetRecentLogs() [][]byte {
	listenersMu.RLock()
	defer listenersMu.RUnlock()
	return append([][]byte{}, logBuffer...)
}

type broadcastHandler struct {
	slog.Handler
	attrs []slog.Attr
}

func (h *broadcastHandler) Handle(ctx context.Context, r slog.Record) error {
	// Standard handle
	err := h.Handler.Handle(ctx, r)

	// Broadcast & Buffer
	listenersMu.Lock()
	defer listenersMu.Unlock()

	var buf strings.Builder
	// Create a JSON handler that already has the attributes from WithAttrs
	var h2 slog.Handler = slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: RedactAttr})
	if len(h.attrs) > 0 {
		h2 = h2.WithAttrs(h.attrs)
	}

	if err := h2.Handle(ctx, r); err == nil {
		msg := []byte(buf.String())

		// Add to buffer
		logBuffer = append(logBuffer, msg)
		if len(logBuffer) > bufferSize {
			logBuffer = logBuffer[1:]
		}

		if len(logListeners) > 0 {
			for _, l := range logListeners {
				select {
				case l <- msg:
				default:
					// drop if full
				}
			}
		}
	}
	return err
}

func (h *broadcastHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &broadcastHandler{
		Handler: h.Handler.WithAttrs(attrs),
		attrs:   append(append([]slog.Attr{}, h.attrs...), attrs...),
	}
}

func (h *broadcastHandler) WithGroup(name string) slog.Handler {
	// For simplicity, we'll just ignore groups in the broadcast for now or handle them simply.
	// Groups are more complex to merge.
	return &broadcastHandler{
		Handler: h.Handler.WithGroup(name),
		attrs:   h.attrs,
	}
}

// Init creates the root logger based on APP_ENV and LOG_LEVEL.
func Init() *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	handler := buildHandler(level)

	// Wrap with broadcast handler
	bHandler := &broadcastHandler{
		Handler: handler,
	}

	logger := slog.New(bHandler)
	slog.SetDefault(logger)
	return logger
}

func buildHandler(level slog.Level) slog.Handler {
	output := io.Writer(os.Stdout)
	if strings.ToLower(os.Getenv("LOG_TO_STDERR")) == "true" {
		output = os.Stderr
	}

	return newHandler(output, level, os.Getenv("APP_ENV") == "production")
}

func newHandler(output io.Writer, level slog.Level, production bool) slog.Handler {
	opts := &slog.HandlerOptions{
		Level:       level,
		AddSource:   level == slog.LevelDebug,
		ReplaceAttr: RedactAttr,
	}

	if production {
		return slog.NewJSONHandler(output, opts)
	}
	return slog.NewTextHandler(output, opts)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Component returns a logger pre-tagged with the component name.
func Component(root *slog.Logger, name string) *slog.Logger {
	envKey := "LOG_LEVEL_" + strings.ToUpper(name)
	if override := os.Getenv(envKey); override != "" {
		level := parseLevel(override)
		// Wrap with a level-overriding handler for this component
		return slog.New(&componentHandler{
			Handler: root.Handler(),
			level:   level,
		}).With(slog.String("component", name))
	}
	return root.With(slog.String("component", name))
}

// NewRequestID generates a randomized request identifier string.
func NewRequestID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("req_%x", b)
}

// componentHandler is a simple wrapper to allow per-component levels (optional)
type componentHandler struct {
	slog.Handler
	level slog.Level
}

func (h *componentHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}
