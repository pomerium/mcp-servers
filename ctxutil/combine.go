package ctxutil

import (
	"context"
	"net/http"
)

func Combine(
	funcs ...func(ctx context.Context, r *http.Request) context.Context,
) func(ctx context.Context, r *http.Request) context.Context {
	return func(ctx context.Context, r *http.Request) context.Context {
		for _, fn := range funcs {
			ctx = fn(ctx, r)
		}
		return ctx
	}
}
