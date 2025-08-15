package logger

import (
	"log/slog"
	"os"
	"strings"
)

var Logger *slog.Logger

func init() {
	level := getLogLevelFromEnv()
	format := getLogFormatFromEnv()
	
	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	
	Logger = slog.New(handler)
}

func getLogLevelFromEnv() slog.Level {
	level := strings.ToLower(os.Getenv("TUF_LOG_LEVEL"))
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getLogFormatFromEnv() string {
	format := strings.ToLower(os.Getenv("TUF_LOG_FORMAT"))
	if format == "json" {
		return "json"
	}
	return "text"
}

func SetLevel(level slog.Level) {
	format := getLogFormatFromEnv()
	opts := &slog.HandlerOptions{
		Level: level,
	}
	
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	
	Logger = slog.New(handler)
}

func SetJSONFormat() {
	level := getLogLevelFromEnv()
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	Logger = slog.New(handler)
}

func SetTextFormat() {
	level := getLogLevelFromEnv()
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewTextHandler(os.Stdout, opts)
	Logger = slog.New(handler)
}