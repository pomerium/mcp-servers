package whoami

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pomerium/mcp-servers/ctxutil"
	"github.com/pomerium/mcp-servers/mcputil"
)

func NewServer(_ context.Context, _ map[string]string) (*server.MCPServer, error) {
	mcpServer := server.NewMCPServer(
		"pomerium-whoami",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
		server.WithRecovery(),
	)

	readQueryTool := mcp.NewTool(
		"whoami",
		mcp.WithDescription("Returns the identity of the user making the request"),
	)
	mcpServer.AddTool(readQueryTool, whoamiHandler)

	return mcpServer, nil
}

func whoamiHandler(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	identity, ok := ctxutil.IdentityFromContext(ctx)
	if !ok {
		return mcp.NewToolResultError("no identity was passed in the request context"), nil
	}

	return mcputil.Response(struct{ Name, Email string }{Name: identity.Name, Email: identity.Email}), nil
}
