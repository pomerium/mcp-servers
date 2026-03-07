package pomerium

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// DynamicCaller makes raw Connect unary HTTP calls using JSON encoding.
// This avoids needing typed Connect clients — we pass JSON straight through
// since the MCP tool input/output matches the protobuf JSON representation.
type DynamicCaller struct {
	httpClient *http.Client
	baseURL    string
}

// NewDynamicCaller creates a caller that uses the given HTTP transport
// (which should include auth headers via authTransport).
func NewDynamicCaller(baseURL string, transport http.RoundTripper) *DynamicCaller {
	return &DynamicCaller{
		httpClient: &http.Client{Transport: transport},
		baseURL:    baseURL,
	}
}

// Call executes a Connect unary RPC using JSON encoding.
// inputJSON is the raw JSON from MCP tool arguments (matches the protobuf JSON schema).
// Returns the response as JSON bytes.
func (c *DynamicCaller) Call(
	ctx context.Context,
	method protoreflect.MethodDescriptor,
	inputJSON json.RawMessage,
) (json.RawMessage, error) {
	// Build the Connect endpoint URL: baseURL/package.Service/Method
	url := c.baseURL + "/" + string(method.Parent().FullName()) + "/" + string(method.Name())

	if len(inputJSON) == 0 {
		inputJSON = []byte("{}")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(inputJSON))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Connect protocol headers for JSON unary
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MiB
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Connect returns non-200 with JSON error body on failure
	if resp.StatusCode != http.StatusOK {
		return nil, parseConnectError(resp.StatusCode, body)
	}

	return body, nil
}

func parseConnectError(statusCode int, body []byte) error {
	var connectErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &connectErr); err == nil && connectErr.Message != "" {
		return fmt.Errorf("%s: %s", connectErr.Code, connectErr.Message)
	}
	return fmt.Errorf("HTTP %d: %s", statusCode, string(body))
}
