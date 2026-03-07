package pomerium

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pomerium/sdk-go/proto/pomerium"
)

// NewServer creates a new MCP server that exposes Pomerium ConfigService
// methods as tools, auto-discovered via protobuf reflection.
//
// Required env vars (after POMERIUM_ prefix stripping by builder):
//   - API_URL:  base URL of the Pomerium API
//   - API_TOKEN: service account token or base64 shared secret
//
// Optional:
//   - TLS_INSECURE_SKIP_VERIFY: skip TLS verification (default false)
func NewServer(_ context.Context, env map[string]string) (*mcp.Server, error) {
	apiURL := env["API_URL"]
	if apiURL == "" {
		return nil, fmt.Errorf("POMERIUM_API_URL is required")
	}
	apiToken := env["API_TOKEN"]
	if apiToken == "" {
		return nil, fmt.Errorf("POMERIUM_API_TOKEN is required")
	}

	var tlsSkipVerify bool
	if v := env["TLS_INSECURE_SKIP_VERIFY"]; v != "" {
		var err error
		tlsSkipVerify, err = strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid POMERIUM_TLS_INSECURE_SKIP_VERIFY value %q: %w", v, err)
		}
	}

	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("http.DefaultTransport is not *http.Transport")
	}
	httpTransport := baseTransport.Clone()
	httpTransport.ForceAttemptHTTP2 = true
	if tlsSkipVerify {
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	authRT := NewAuthTransport(
		AuthConfig{APIToken: apiToken, BaseURL: apiURL},
		httpTransport,
	)

	caller := NewDynamicCaller(apiURL, authRT)

	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "pomerium",
			Version: "1.0.0",
		},
		nil,
	)

	RegisterTools(mcpServer, caller, pomerium.File_config_proto)

	return mcpServer, nil
}
