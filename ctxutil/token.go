package ctxutil

import (
	"context"
	"fmt"
	"net/http"
)

type authKey struct{}

// TokenFromRequest extracts the authorization token from the HTTP request
func TokenFromRequest(ctx context.Context, r *http.Request) context.Context {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ctx
	}
	prefix := "Bearer "
	if len(auth) > len(prefix) && auth[:len(prefix)] == prefix {
		return context.WithValue(ctx, authKey{}, auth[len(prefix):])
	}
	return ctx
}

// TokenFromContext retrieves the authorization token from the context
func TokenFromContext(ctx context.Context) (string, error) {
	auth, ok := ctx.Value(authKey{}).(string)
	if !ok {
		return "", fmt.Errorf("missing auth")
	}
	return auth, nil
}
