package drutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider implements Provider for testing
type mockProvider struct {
	searchSyntax string
	searchErr    error
	fetchErr     error
}

func (m *mockProvider) GetSearchSyntax() string {
	return m.searchSyntax
}

func (m *mockProvider) Search(ctx context.Context, query string) ([]Document, error) {
	return nil, m.searchErr
}

func (m *mockProvider) Fetch(ctx context.Context, id string) (*Document, error) {
	return nil, m.fetchErr
}

func TestGetTools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		searchSyntax string
		searchErr    error
		fetchErr     error
		want         []ToolDefinition
		checkErr     bool
	}{
		{
			name:         "normal case",
			searchSyntax: "search by page title",
			want: []ToolDefinition{
				{
					Name:        "search",
					Description: "search by page title",
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
			},
		},
		{
			name:         "empty search syntax",
			searchSyntax: "",
			want: []ToolDefinition{
				{
					Name:        "search",
					Description: "",
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
			},
		},
		{
			name:         "search error",
			searchSyntax: "search by page title",
			searchErr:    assert.AnError,
			want: []ToolDefinition{
				{
					Name:        "search",
					Description: "search by page title",
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
			},
		},
		{
			name:         "fetch error",
			searchSyntax: "search by page title",
			fetchErr:     assert.AnError,
			want: []ToolDefinition{
				{
					Name:        "search",
					Description: "search by page title",
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
			},
		},
		{
			name:         "both errors",
			searchSyntax: "search by page title",
			searchErr:    assert.AnError,
			fetchErr:     assert.AnError,
			want: []ToolDefinition{
				{
					Name:        "search",
					Description: "search by page title",
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
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := &mockProvider{
				searchSyntax: tt.searchSyntax,
				searchErr:    tt.searchErr,
				fetchErr:     tt.fetchErr,
			}

			got := GetTools(provider)
			require.Len(t, got, len(tt.want), "should have correct number of tools")

			// Compare each tool
			for i, want := range tt.want {
				assert.Equal(t, want.Name, got[i].Name)
				assert.Equal(t, want.Description, got[i].Description)
				assert.Equal(t, want.Parameters, got[i].Parameters)
			}
		})
	}
}
