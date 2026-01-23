package whoami

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pomerium/mcp-servers/ctxutil"
	"github.com/pomerium/mcp-servers/mcputil"
)

func NewServer(_ context.Context, _ map[string]string) (*mcp.Server, error) {
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "pomerium-whoami",
			Version: "1.0.0",
		},
		nil,
	)

	// Define the tool handler using AddTool
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "whoami",
		Description: "Returns the identity of the user making the request",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		identity, ok := ctxutil.IdentityFromContext(ctx)
		if !ok {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "no identity was passed in the request context"},
				},
				IsError: true,
			}, nil, nil
		}

		return mcputil.Response(struct{ Name, Email string }{Name: identity.Name, Email: identity.Email}), nil, nil
	})

	return mcpServer, nil
}
