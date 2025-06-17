package server

import (
	"encoding/json"
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	"github.com/pomerium/mcp-servers/drutil"
)

// Tool represents an MCP tool definition
type Tool struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	Parameters  map[string]drutil.ToolParameter `json:"parameters"`
}

// ToolsResponse represents the response for the /.well-known/mcp/tools endpoint
type ToolsResponse struct {
	Tools []Tool `json:"tools"`
}

// toolsHandler handles requests to /.well-known/mcp/tools
func toolsHandler(mcpServer *server.MCPServer, p drutil.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get tool definitions
		defs := drutil.GetTools(p)

		// Convert to response format
		tools := make([]Tool, len(defs))
		for i, def := range defs {
			tools[i] = Tool{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.Parameters,
			}
		}

		response := ToolsResponse{
			Tools: tools,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Error encoding response", http.StatusInternalServerError)
			return
		}
	}
}
