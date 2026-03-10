package common_slog

import (
	"context"
	"log/slog"
)

const key = "common_slog"

func FromContext(ctx context.Context) *slog.Logger {
	ctxSlog := ctx.Value(key)
	if ctxSlog == nil {
		return slog.Default()
	}
	return ctxSlog.(*slog.Logger)
}

func NewContext(ctx context.Context, slog *slog.Logger) context.Context {
	return context.WithValue(ctx, key, slog)
}
