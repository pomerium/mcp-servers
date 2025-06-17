package drutil

// ToolParameter represents a parameter for a tool
type ToolParameter struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolDefinition represents a tool's definition
type ToolDefinition struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Parameters  map[string]ToolParameter `json:"parameters"`
}

// GetTools returns the list of available tools
func GetTools(p Provider) []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "search",
			Description: p.GetSearchSyntax(),
			Parameters: map[string]ToolParameter{
				"query": {
					Type:        "string",
					Description: "The search query to execute",
					Required:    true,
				},
			},
		},
		{
			Name:        "fetch",
			Description: "Fetch a document by ID",
			Parameters: map[string]ToolParameter{
				"id": {
					Type:        "string",
					Description: "The ID of the document to fetch",
					Required:    true,
				},
			},
		},
	}
}
