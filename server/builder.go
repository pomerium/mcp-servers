package server

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/mark3labs/mcp-go/server"

	"github.com/pomerium/mcp-servers/ctxutil"
	"github.com/pomerium/mcp-servers/notion"
	"github.com/pomerium/mcp-servers/sqlite"
	"github.com/pomerium/mcp-servers/whoami"
	"github.com/pomerium/sdk-go"
)

func BuildHandlers(ctx context.Context) http.Handler {
	mux := http.NewServeMux()

	for name, builder := range map[string]func(
		ctx context.Context,
		env map[string]string,
	) (*server.MCPServer, error){
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
		httpHandler := server.NewStreamableHTTPServer(mcpServer,
			server.WithHTTPContextFunc(
				ctxutil.Combine(
					ctxutil.AuthorizationTokenFromRequest,
					ctxutil.NewVerifier(v).IdentityFromRequest,
				),
			),
			server.WithStateLess(true),
		)
		mux.Handle(path.Join("/", name), http.HandlerFunc(httpHandler.ServeHTTP))
	}

	return mux
}
