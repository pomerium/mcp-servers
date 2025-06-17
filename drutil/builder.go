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

	// Get tool definitions
	tools := GetTools(p)

	// Create and register tools
	for _, def := range tools {
		switch def.Name {
		case "search":
			tool := mcp.NewTool(
				def.Name,
				mcp.WithDescription(def.Description),
				mcp.WithString("query",
					mcp.Required(),
					mcp.Description("The search query to execute"),
				),
			)
			server.AddTool(tool, searchHandler(p))
		case "fetch":
			tool := mcp.NewTool(
				def.Name,
				mcp.WithDescription(def.Description),
				mcp.WithString("id",
					mcp.Required(),
					mcp.Description("The ID of the document to fetch"),
				),
			)
			server.AddTool(tool, fetchHandler(p))
		}
	}

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
