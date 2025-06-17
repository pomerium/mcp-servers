package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/pomerium/mcp-servers/drutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider implements drutil.Provider for testing
type mockProvider struct {
	searchSyntax string
}

func (m *mockProvider) GetSearchSyntax() string {
	return m.searchSyntax
}

func (m *mockProvider) Search(ctx context.Context, query string) ([]drutil.Document, error) {
	return nil, nil
}

func (m *mockProvider) Fetch(ctx context.Context, id string) (*drutil.Document, error) {
	return nil, nil
}

func TestToolsHandler(t *testing.T) {
	t.Parallel()

	// Create a mock MCP server
	mcpServer := server.NewMCPServer(
		"test",
		"0.0.1",
		server.WithToolCapabilities(true),
	)

	tests := []struct {
		name          string
		method        string
		searchSyntax  string
		wantStatus    int
		wantTools     []Tool
		checkResponse bool
		path          string
		headers       map[string]string
	}{
		{
			name:         "GET request with normal provider",
			method:       http.MethodGet,
			searchSyntax: "search by page title",
			wantStatus:   http.StatusOK,
			wantTools: []Tool{
				{
					Name:        "search",
					Description: "search by page title",
					Parameters: map[string]drutil.ToolParameter{
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
					Parameters: map[string]drutil.ToolParameter{
						"id": {
							Type:        "string",
							Description: "The ID of the document to fetch",
							Required:    true,
						},
					},
				},
			},
			checkResponse: true,
			path:          "/.well-known/mcp/tools",
		},
		{
			name:         "GET request with empty search syntax",
			method:       http.MethodGet,
			searchSyntax: "",
			wantStatus:   http.StatusOK,
			wantTools: []Tool{
				{
					Name:        "search",
					Description: "",
					Parameters: map[string]drutil.ToolParameter{
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
					Parameters: map[string]drutil.ToolParameter{
						"id": {
							Type:        "string",
							Description: "The ID of the document to fetch",
							Required:    true,
						},
					},
				},
			},
			checkResponse: true,
			path:          "/.well-known/mcp/tools",
		},
		{
			name:          "POST request",
			method:        http.MethodPost,
			searchSyntax:  "search by page title",
			wantStatus:    http.StatusMethodNotAllowed,
			checkResponse: false,
			path:          "/.well-known/mcp/tools",
		},
		{
			name:          "PUT request",
			method:        http.MethodPut,
			searchSyntax:  "search by page title",
			wantStatus:    http.StatusMethodNotAllowed,
			checkResponse: false,
			path:          "/.well-known/mcp/tools",
		},
		{
			name:         "invalid path",
			method:       http.MethodGet,
			searchSyntax: "search by page title",
			wantStatus:   http.StatusOK,
			wantTools: []Tool{
				{
					Name:        "search",
					Description: "search by page title",
					Parameters: map[string]drutil.ToolParameter{
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
					Parameters: map[string]drutil.ToolParameter{
						"id": {
							Type:        "string",
							Description: "The ID of the document to fetch",
							Required:    true,
						},
					},
				},
			},
			checkResponse: true,
			path:          "/invalid/path",
		},
		{
			name:         "malformed JSON request",
			method:       http.MethodGet,
			searchSyntax: "search by page title",
			wantStatus:   http.StatusOK,
			wantTools: []Tool{
				{
					Name:        "search",
					Description: "search by page title",
					Parameters: map[string]drutil.ToolParameter{
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
					Parameters: map[string]drutil.ToolParameter{
						"id": {
							Type:        "string",
							Description: "The ID of the document to fetch",
							Required:    true,
						},
					},
				},
			},
			checkResponse: true,
			path:          "/.well-known/mcp/tools",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name:         "unsupported content type",
			method:       http.MethodGet,
			searchSyntax: "search by page title",
			wantStatus:   http.StatusOK,
			wantTools: []Tool{
				{
					Name:        "search",
					Description: "search by page title",
					Parameters: map[string]drutil.ToolParameter{
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
					Parameters: map[string]drutil.ToolParameter{
						"id": {
							Type:        "string",
							Description: "The ID of the document to fetch",
							Required:    true,
						},
					},
				},
			},
			checkResponse: true,
			path:          "/.well-known/mcp/tools",
			headers: map[string]string{
				"Content-Type": "text/plain",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := &mockProvider{searchSyntax: tt.searchSyntax}
			req := httptest.NewRequest(tt.method, tt.path, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()

			handler := toolsHandler(mcpServer, provider)
			handler.ServeHTTP(w, req)

			// Check status code
			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.checkResponse {
				// Check response
				var response ToolsResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err, "should decode response JSON")

				// Check tools
				require.Len(t, response.Tools, len(tt.wantTools), "should have correct number of tools")

				// Compare each tool
				for i, want := range tt.wantTools {
					assert.Equal(t, want.Name, response.Tools[i].Name)
					assert.Equal(t, want.Description, response.Tools[i].Description)
					assert.Equal(t, want.Parameters, response.Tools[i].Parameters)
				}
			}
		})
	}
}
