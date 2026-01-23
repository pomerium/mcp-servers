package mcputil

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func Response[T any](v T) *mcp.CallToolResult {
	resultJSON, err := json.Marshal(v)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Error formatting response: " + err.Error()},
			},
			IsError: true,
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(resultJSON)},
		},
	}
}
