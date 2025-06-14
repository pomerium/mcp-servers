package drutil

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func BuildMCPServer(
	name string,
	p Provider,
) *server.MCPServer {
	server := server.NewMCPServer(
		name,
		"0.0.1",
		server.WithToolCapabilities(true),
		server.WithLogging(),
		server.WithRecovery(),
	)

	search := mcp.NewTool(
		"search",
		mcp.WithDescription(p.GetSearchSyntax()),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The search query to execute"),
		),
	)

	fetch := mcp.NewTool(
		"fetch",
		mcp.WithDescription("Fetch a document by ID"),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("The ID of the document to fetch"),
		),
	)

	server.AddTool(search, searchHandler(p))
	server.AddTool(fetch, fetchHandler(p))

	return server
}

func searchHandler(p Provider) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		query, ok := args["query"].(string)
		if !ok || query == "" {
			return mcp.NewToolResultError("Missing or empty 'query' argument"), nil
		}

		documents, err := p.Search(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}

		result := struct {
			Results []Document `json:"results"`
		}{
			Results: documents,
		}
		data, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("marshal search results: %w", err)
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

func fetchHandler(p Provider) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()
		id, ok := args["id"].(string)
		if !ok || id == "" {
			return mcp.NewToolResultError("Missing or empty 'id' argument"), nil
		}

		document, err := p.Fetch(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("fetch: %w", err)
		}

		data, err := json.Marshal(document)
		if err != nil {
			return nil, fmt.Errorf("marshal document: %w", err)
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}
