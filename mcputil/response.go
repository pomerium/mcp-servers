package mcputil

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

func Response[T any](v T) *mcp.CallToolResult {
	resultJSON, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Error formatting response", err)
	}

	return mcp.NewToolResultText(string(resultJSON))
}
