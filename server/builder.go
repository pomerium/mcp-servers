package server

import (
	"context"
	"net/http"
	"path"

	"github.com/mark3labs/mcp-go/server"

	"github.com/pomerium/mcp-servers/ctxutil"
	"github.com/pomerium/mcp-servers/drutil"
	"github.com/pomerium/mcp-servers/notion"
)

func BuildHandlers(ctx context.Context) http.Handler {
	mux := http.NewServeMux()

	for name, builder := range map[string]drutil.Provider{
		"notion": notion.New(ctx),
	} {
		mcpServer := drutil.BuildMCPServer(name, builder)
		httpHandler := server.NewStreamableHTTPServer(mcpServer,
			server.WithHTTPContextFunc(ctxutil.TokenFromRequest),
		)
		mux.Handle(path.Join("/", name), http.HandlerFunc(httpHandler.ServeHTTP))

		mux.Handle(path.Join("/", name, ".well-known", "mcp", "tools"), toolsHandler(mcpServer, builder))
	}

	return mux
}
