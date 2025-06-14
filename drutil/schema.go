// Package drutil provides utility functions for OpenAI DeepResearcher compatibility
package drutil

import "context"

type Document struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Text     string            `json:"text"`
	URL      *string           `json:"url,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Provider interface {
	GetSearchSyntax() string
	Search(ctx context.Context, query string) ([]Document, error)
	Fetch(ctx context.Context, id string) (*Document, error)
}
