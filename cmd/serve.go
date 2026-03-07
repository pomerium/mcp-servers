package main

import (
	"context"
	"os"

	"github.com/pomerium/mcp-servers/httputil"
	"github.com/pomerium/mcp-servers/server"
)

func serveCommand(ctx context.Context, args []string) error {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	handler := server.BuildHandlers(ctx, args)
	return httputil.ListenAndServe(ctx, addr, handler)
}
