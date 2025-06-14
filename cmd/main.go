package main

import (
	"context"
	"log"

	"github.com/pomerium/mcp-servers/httputil"
	"github.com/pomerium/mcp-servers/server"
)

func main() {
	err := run(context.Background())
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	handler := server.BuildHandlers(ctx)
	return httputil.ListenAndServe(ctx, ":3010", handler)
}
