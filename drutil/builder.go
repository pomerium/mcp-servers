package drutil

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func BuildMCPServer(
	name string,
	p Provider,
) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    name,
			Version: "0.0.1",
		},
		nil,
	)

	// Define search tool input/output types
	type searchArgs struct {
		Query string `json:"query" jsonschema:"The search query to execute"`
	}
	type searchResult struct {
		Results []Document `json:"results"`
	}

	// Define fetch tool input/output types
	type fetchArgs struct {
		ID string `json:"id" jsonschema:"The ID of the document to fetch"`
	}

	// Add search tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: p.GetSearchSyntax(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, searchResult, error) {
		documents, err := p.Search(ctx, args.Query)
		if err != nil {
			return nil, searchResult{}, fmt.Errorf("search: %w", err)
		}
		return nil, searchResult{Results: documents}, nil
	})

	// Add fetch tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "fetch",
		Description: "Fetch a document by ID",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args fetchArgs) (*mcp.CallToolResult, *Document, error) {
		document, err := p.Fetch(ctx, args.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch: %w", err)
		}
		return nil, document, nil
	})

	return server
}
