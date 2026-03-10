package logging

import (
	"log"
	"log/slog"
	"os"
)

// Setup initializes structured logging using slog.
// It sets slog as the default logger and bridges the standard log package
// so existing log.Printf calls output structured JSON in production.
func Setup() {
	env := os.Getenv("LOG_FORMAT")

	var handler slog.Handler
	if env == "text" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		// Default: JSON structured logging
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Bridge standard log package to slog
	log.SetFlags(0)
	log.SetOutput(&slogWriter{logger: logger})
}

// slogWriter adapts slog.Logger to io.Writer for the standard log package.
type slogWriter struct {
	logger *slog.Logger
}

func (w *slogWriter) Write(p []byte) (n int, err error) {
	// Trim trailing newline added by log package
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	w.logger.Info(msg)
	return len(p), nil
}
