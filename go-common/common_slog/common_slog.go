package common_slog

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

var key = ctxKey{}

func FromContext(ctx context.Context) *slog.Logger {
	ctxSlog := ctx.Value(key)
	if ctxSlog == nil {
		return slog.Default()
	}
	return ctxSlog.(*slog.Logger)
}

func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, key, logger)
}
