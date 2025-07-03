package ctxutil

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/pomerium/sdk-go"
)

type identityKey struct{}

type Verifier struct {
	*sdk.Verifier
}

func NewVerifier(verifier *sdk.Verifier) *Verifier {
	return &Verifier{
		Verifier: verifier,
	}
}

// IdentityFromRequest returns a new context with the identity token extracted from the HTTP request.
func (v *Verifier) IdentityFromRequest(ctx context.Context, r *http.Request) context.Context {
	jwt := r.Header.Get("x-pomerium-jwt-assertion")
	if jwt == "" {
		slog.Error("no JWT assertion header found in request")
		return ctx
	}
	identity, err := v.Verifier.GetIdentity(ctx, jwt)
	if err != nil {
		slog.Error("failed to get identity from JWT assertion", "error", err)
		return ctx
	}
	return context.WithValue(ctx, identityKey{}, identity)
}

// IdentityFromContext retrieves the user identity from the context.
func IdentityFromContext(ctx context.Context) (*sdk.Identity, bool) {
	v, ok := ctx.Value(identityKey{}).(*sdk.Identity)
	return v, ok
}
