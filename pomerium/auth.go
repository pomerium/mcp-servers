package pomerium

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
)

// AuthConfig holds the configuration for authenticating with the Pomerium API.
type AuthConfig struct {
	APIToken string // service account token or base64-encoded shared secret
	BaseURL  string
}

// serverType mirrors pomerium.ServerType enum values.
type serverType int32

const (
	serverTypeUnknown    serverType = 0
	serverTypeCore       serverType = 1
	serverTypeEnterprise serverType = 2
	serverTypeZero       serverType = 3

	// bootstrapID is the well-known UUID used for bootstrap service accounts.
	bootstrapID = "014e587b-3f4b-4fcf-90a9-f6ecdf8154af"
)

type authTransport struct {
	cfg       AuthConfig
	inner     http.RoundTripper
	sharedKey []byte // parsed once at construction; nil if token is not a shared key

	mu          sync.Mutex
	svrType     serverType
	svrTypeInit bool
}

// NewAuthTransport wraps an http.RoundTripper with Pomerium authentication.
// It replicates the auth logic from github.com/pomerium/sdk-go client.go.
func NewAuthTransport(cfg AuthConfig, inner http.RoundTripper) http.RoundTripper {
	t := &authTransport{
		cfg:   cfg,
		inner: inner,
	}
	t.sharedKey, _ = parseTokenAsKey(cfg.APIToken)
	return t
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid mutating the original
	req = req.Clone(req.Context())

	// For GetServerInfo, use unknown server type (avoid recursion)
	var st serverType
	if req.URL.Path == "/pomerium.config.ConfigService/GetServerInfo" {
		st = serverTypeUnknown
	} else {
		var err error
		st, err = t.getServerType(req.Context())
		if err != nil {
			return nil, fmt.Errorf("determining server type: %w", err)
		}
	}

	if err := t.setAuthHeaders(req.Header, st); err != nil {
		return nil, err
	}

	return t.inner.RoundTrip(req)
}

func (t *authTransport) getServerType(ctx context.Context) (serverType, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.svrTypeInit {
		return t.svrType, nil
	}

	st, err := t.fetchServerType(ctx)
	if err != nil {
		return serverTypeUnknown, err
	}
	t.svrType = st
	t.svrTypeInit = true
	return st, nil
}

func (t *authTransport) fetchServerType(ctx context.Context) (serverType, error) {
	body := []byte("{}")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.BaseURL+"/pomerium.config.ConfigService/GetServerInfo", bytes.NewReader(body))
	if err != nil {
		return serverTypeUnknown, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if err := t.setAuthHeaders(req.Header, serverTypeUnknown); err != nil {
		return serverTypeUnknown, err
	}

	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return serverTypeUnknown, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB
	if err != nil {
		return serverTypeUnknown, err
	}

	if resp.StatusCode != http.StatusOK {
		// If 415 Unsupported Media Type, it's a legacy enterprise server
		if resp.StatusCode == http.StatusUnsupportedMediaType {
			return serverTypeEnterprise, nil
		}
		return serverTypeUnknown, fmt.Errorf("GetServerInfo returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ServerType string `json:"serverType"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return serverTypeUnknown, fmt.Errorf("parsing GetServerInfo response: %w", err)
	}

	switch result.ServerType {
	case "SERVER_TYPE_CORE":
		return serverTypeCore, nil
	case "SERVER_TYPE_ENTERPRISE":
		return serverTypeEnterprise, nil
	case "SERVER_TYPE_ZERO":
		return serverTypeZero, nil
	default:
		return serverTypeUnknown, nil
	}
}

func (t *authTransport) setAuthHeaders(headers http.Header, st serverType) error {
	token := t.cfg.APIToken

	// If this is a zero server, we'd need token exchange.
	// For now, support core/enterprise auth patterns.
	if st == serverTypeZero {
		// Zero uses "Pomerium <id_token>" after token exchange.
		// Token exchange is not yet implemented; using raw token.
		slog.Warn("Pomerium Zero token exchange is not yet implemented; using raw token")
		headers.Set("Authorization", "Pomerium "+token)
		return nil
	}

	// Use pre-parsed shared key if available
	if t.sharedKey != nil {
		bootstrapJWT, err := generateBootstrapJWT(t.sharedKey)
		if err != nil {
			return fmt.Errorf("generating bootstrap JWT: %w", err)
		}
		headers.Set("Authorization", "Pomerium "+bootstrapJWT)

		// Core and unknown server types also need the jwt header
		if st == serverTypeCore || st == serverTypeUnknown {
			coreJWT, err := generateGRPCJWT(t.sharedKey)
			if err != nil {
				return fmt.Errorf("generating core JWT: %w", err)
			}
			headers["jwt"] = []string{coreJWT}
		}
	} else {
		// Raw service account token
		headers.Set("Authorization", "Pomerium "+token)
		headers["jwt"] = []string{token}
	}

	return nil
}

// parseTokenAsKey checks if the token is a base64-encoded 32-byte key.
// Tries standard and URL-safe base64 encodings (with and without padding).
func parseTokenAsKey(str string) (key []byte, ok bool) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		key, err := enc.DecodeString(str)
		if err == nil && len(key) == 32 {
			return key, true
		}
	}
	return nil, false
}

// signJWT creates an HS256-signed JWT with the given claims.
func signJWT(key []byte, claims jwt.Claims) (string, error) {
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return "", err
	}
	return jwt.Signed(sig).Claims(claims).CompactSerialize()
}

// generateGRPCJWT generates a JWT for databroker authentication.
func generateGRPCJWT(key []byte) (string, error) {
	return signJWT(key, jwt.Claims{
		Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
}

// generateBootstrapJWT generates a bootstrap service account JWT.
func generateBootstrapJWT(key []byte) (string, error) {
	now := time.Now()
	return signJWT(key, jwt.Claims{
		ID:        bootstrapID,
		Subject:   "bootstrap-" + bootstrapID + ".pomerium",
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
	})
}
