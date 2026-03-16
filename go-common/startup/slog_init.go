package startup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/m-szalik/goutils"
)

func InitSlog() error {
	var logLevel slog.Level
	switch ll := strings.ToUpper(strings.TrimSpace(goutils.Env("LOG_LEVEL", "INFO"))); ll {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN", "WARNING":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		return fmt.Errorf("invalid LOG_LEVEL env value '%s'", ll)
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
	if logLevel > slog.LevelInfo {
		slog.Default().Log(context.Background(), logLevel, "Log level set", "level", logLevel)
	} else {
		slog.Default().Log(context.Background(), slog.LevelInfo, "Log level set", "level", logLevel)
	}
	return nil
}
