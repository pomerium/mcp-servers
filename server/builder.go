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
)

func BuildHandlers(ctx context.Context) http.Handler {
	mux := http.NewServeMux()

	for name, builder := range map[string]func(
		ctx context.Context,
		env map[string]string,
	) (*server.MCPServer, error){
		"notion": notion.NewServer,
		"sqlite": sqlite.NewServer,
	} {
		mcpServer, err := builder(ctx, getEnvByPrefix(strings.ToUpper(name)+"_"))
		if err != nil {
			slog.Error("Not enabling", "name", name, "error", err)
			continue
		}
		slog.Info("Enabled", "name", name)
		httpHandler := server.NewStreamableHTTPServer(mcpServer,
			server.WithHTTPContextFunc(ctxutil.TokenFromRequest),
		)
		mux.Handle(path.Join("/", name), http.HandlerFunc(httpHandler.ServeHTTP))
	}

	return mux
}
