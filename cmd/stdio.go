package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pomerium/mcp-servers/server"
)

func stdioCommand(ctx context.Context, args []string) error {
	var name string
	if len(args) > 0 {
		name = args[0]
	}
	if name == "" {
		name = os.Getenv("SERVER")
	}
	if name == "" {
		return fmt.Errorf("usage: %s stdio <server-name>\n\nAvailable servers: %v", programName, server.AvailableServers())
	}

	mcpServer, err := server.BuildServer(ctx, name)
	if err != nil {
		return fmt.Errorf("building server %q: %w", name, err)
	}

	return mcpServer.Run(ctx, &mcp.StdioTransport{})
}
