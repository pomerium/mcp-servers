package main

import (
	"context"
	"os"

	"github.com/pomerium/mcp-servers/httputil"
	"github.com/pomerium/mcp-servers/server"
)

func serveCommand(ctx context.Context, _ []string) error {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	handler := server.BuildHandlers(ctx)
	return httputil.ListenAndServe(ctx, addr, handler)
}
