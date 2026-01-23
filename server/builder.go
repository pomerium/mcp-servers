package server

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pomerium/mcp-servers/ctxutil"
	"github.com/pomerium/mcp-servers/notion"
	"github.com/pomerium/mcp-servers/sqlite"
	"github.com/pomerium/mcp-servers/whoami"
	"github.com/pomerium/sdk-go"
)

// contextKey is a type for context keys to avoid collisions
type contextKey string

const httpRequestKey contextKey = "http_request"

func BuildHandlers(ctx context.Context) http.Handler {
	mux := http.NewServeMux()

	for name, builder := range map[string]func(
		ctx context.Context,
		env map[string]string,
	) (*mcp.Server, error){
		"notion": notion.NewServer,
		"sqlite": sqlite.NewServer,
		"whoami": whoami.NewServer,
	} {
		v, err := sdk.New(&sdk.Options{})
		if err != nil {
			slog.Error("Failed to create SDK verifier", "name", name, "error", err)
			continue
		}

		mcpServer, err := builder(ctx, getEnvByPrefix(strings.ToUpper(name)+"_"))
		if err != nil {
			slog.Error("Not enabling", "name", name, "error", err)
			continue
		}
		slog.Info("Enabled", "name", name)

		// Create a streamable HTTP handler
		httpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			// Store the request in a context that tool handlers can access
			// This will be done through a wrapper in the transport layer
			return mcpServer
		}, &mcp.StreamableHTTPOptions{
			Stateless: true,
		})

		// Wrap the handler to add authentication context
		wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add the HTTP request to the context so tool handlers can access it
			ctx := context.WithValue(r.Context(), httpRequestKey, r)
			// Apply context transformations from Pomerium SDK
			ctx = ctxutil.Combine(
				ctxutil.AuthorizationTokenFromRequest,
				ctxutil.NewVerifier(v).IdentityFromRequest,
			)(ctx, r)
			r = r.WithContext(ctx)
			httpHandler.ServeHTTP(w, r)
		})

		mux.Handle(path.Join("/", name), wrappedHandler)
	}

	return mux
}
