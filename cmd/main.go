package main

import (
	"context"
	"log"
	"os"

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
	port, ok := os.LookupEnv("PORT")
	if !ok {
		port = "8080"
	}
	handler := server.BuildHandlers(ctx)
	return httputil.ListenAndServe(ctx, ":"+port, handler)
}
