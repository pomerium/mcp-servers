package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pomerium/mcp-servers/ctxutil"
	"github.com/pomerium/mcp-servers/notion"
	pomeriumpkg "github.com/pomerium/mcp-servers/pomerium"
	"github.com/pomerium/mcp-servers/sqlite"
	"github.com/pomerium/mcp-servers/whoami"
	"github.com/pomerium/sdk-go"
)

// contextKey is a type for context keys to avoid collisions
type contextKey string

const httpRequestKey contextKey = "http_request"

var serverBuilders = map[string]func(
	ctx context.Context,
	env map[string]string,
) (*mcp.Server, error){
	"notion":   notion.NewServer,
	"sqlite":   sqlite.NewServer,
	"whoami":   whoami.NewServer,
	"pomerium": pomeriumpkg.NewServer,
}

// AvailableServers returns the names of all registered MCP servers.
func AvailableServers() []string {
	names := make([]string, 0, len(serverBuilders))
	for name := range serverBuilders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BuildServer creates a single MCP server by name.
func BuildServer(ctx context.Context, name string) (*mcp.Server, error) {
	builder, ok := serverBuilders[name]
	if !ok {
		return nil, fmt.Errorf("unknown server %q, available: %v", name, AvailableServers())
	}
	return builder(ctx, getEnvByPrefix(strings.ToUpper(name)+"_"))
}

// BuildHandlers creates HTTP handlers for the given server names.
// If names is empty, all registered servers are started.
func BuildHandlers(ctx context.Context, names []string) http.Handler {
	mux := http.NewServeMux()

	builders := serverBuilders
	if len(names) > 0 {
		builders = make(map[string]func(context.Context, map[string]string) (*mcp.Server, error), len(names))
		for _, name := range names {
			b, ok := serverBuilders[name]
			if !ok {
				slog.Error("Unknown server", "name", name, "available", AvailableServers())
				continue
			}
			builders[name] = b
		}
	}

	for name, builder := range builders {
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
