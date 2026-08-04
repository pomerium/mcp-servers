package stdioproxy_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pomerium/mcp-servers/stdioproxy"
)

// getFreePort returns an available TCP port.
func getFreePort(t *testing.T) int {
	t.Helper()
	addr, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer addr.Close()
	return addr.Addr().(*net.TCPAddr).Port
}

// startProxy starts the stdio-proxy with an in-process test server.
func startProxy(t *testing.T, port int) (cleanup func()) {
	t.Helper()

	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create in-process test server and get transport
	transport := newTestServer(t)

	// Create ProcessManager with the transport
	pm := stdioproxy.NewProcessManager(stdioproxy.ProcessManagerConfig{
		Transport: transport,
		Logger:    logger,
	})

	// Start the process manager (connects to in-process server)
	if err := pm.Start(ctx); err != nil {
		t.Fatalf("failed to start process manager: %v", err)
	}

	// Create proxy server
	proxyServer := stdioproxy.NewProxyServer(pm, logger)

	// Create HTTP handler
	httpHandler, err := stdioproxy.NewHTTPHandler(pm, proxyServer, logger)
	if err != nil {
		pm.Stop()
		t.Fatalf("failed to create HTTP handler: %v", err)
	}

	// Start HTTP server
	server := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", port),
		Handler: httpHandler,
	}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			// Don't log expected shutdown errors
		}
	}()

	// Wait for server to be ready
	waitForServer(t, port, 5*time.Second)

	return func() {
		server.Close()
		pm.Stop()
	}
}

// waitForServer waits for the HTTP server to be ready.
func waitForServer(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://localhost:%d", port)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			time.Sleep(50 * time.Millisecond)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready within %v", url, timeout)
}

// mcpRequest sends an MCP JSON-RPC request and returns the response.
func mcpRequest(t *testing.T, port int, method string, params any) map[string]any {
	t.Helper()

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	// Parse SSE response if needed
	respStr := string(respBody)
	if strings.HasPrefix(strings.TrimSpace(respStr), "event:") || strings.HasPrefix(strings.TrimSpace(respStr), "data:") {
		var lastData string
		for _, line := range strings.Split(respStr, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				lastData = strings.TrimPrefix(line, "data:")
				lastData = strings.TrimSpace(lastData)
			}
		}
		if lastData != "" {
			respBody = []byte(lastData)
		}
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v\nBody: %s", err, string(respBody))
	}

	return result
}

func TestStdioProxyToolsList(t *testing.T) {
	port := getFreePort(t)
	cleanup := startProxy(t, port)
	defer cleanup()

	resp := mcpRequest(t, port, "tools/list", nil)

	if errVal, ok := resp["error"]; ok {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got: %v", resp)
	}

	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools array, got: %v", result)
	}

	// Check we have our test tools
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolMap := tool.(map[string]any)
		toolNames[toolMap["name"].(string)] = true
	}

	expectedTools := []string{"echo", "add", "slow", "error", "exit"}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("expected tool %q not found in response", name)
		}
	}
}

func TestStdioProxyToolCall(t *testing.T) {
	port := getFreePort(t)
	cleanup := startProxy(t, port)
	defer cleanup()

	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantText string
		wantErr  bool
	}{
		{
			name:     "echo tool",
			tool:     "echo",
			args:     map[string]any{"message": "Hello, World!"},
			wantText: "Hello, World!",
		},
		{
			name:     "add tool",
			tool:     "add",
			args:     map[string]any{"a": 2, "b": 3},
			wantText: `"result":5`,
		},
		{
			name:    "error tool",
			tool:    "error",
			args:    map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := mcpRequest(t, port, "tools/call", map[string]any{
				"name":      tt.tool,
				"arguments": tt.args,
			})

			if errVal, ok := resp["error"]; ok {
				t.Fatalf("unexpected error: %v", errVal)
			}

			result, ok := resp["result"].(map[string]any)
			if !ok {
				t.Fatalf("expected result object, got: %v", resp)
			}

			isError, _ := result["isError"].(bool)
			if isError != tt.wantErr {
				t.Errorf("isError = %v, want %v", isError, tt.wantErr)
			}

			content, ok := result["content"].([]any)
			if !ok || len(content) == 0 {
				t.Fatalf("expected content array, got: %v", result)
			}

			firstContent := content[0].(map[string]any)
			text, _ := firstContent["text"].(string)

			if tt.wantText != "" && !strings.Contains(text, tt.wantText) {
				t.Errorf("response text = %q, want to contain %q", text, tt.wantText)
			}
		})
	}
}

func TestStdioProxyPromptsList(t *testing.T) {
	port := getFreePort(t)
	cleanup := startProxy(t, port)
	defer cleanup()

	resp := mcpRequest(t, port, "prompts/list", nil)

	if errVal, ok := resp["error"]; ok {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got: %v", resp)
	}

	prompts, ok := result["prompts"].([]any)
	if !ok {
		t.Fatalf("expected prompts array, got: %v", result)
	}

	if len(prompts) == 0 {
		t.Error("expected at least one prompt")
	}

	// Check for our greeting prompt
	found := false
	for _, p := range prompts {
		prompt := p.(map[string]any)
		if prompt["name"] == "greeting" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected greeting prompt not found")
	}
}

func TestStdioProxyPromptGet(t *testing.T) {
	port := getFreePort(t)
	cleanup := startProxy(t, port)
	defer cleanup()

	resp := mcpRequest(t, port, "prompts/get", map[string]any{
		"name": "greeting",
		"arguments": map[string]any{
			"name": "Test User",
		},
	})

	if errVal, ok := resp["error"]; ok {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got: %v", resp)
	}

	messages, ok := result["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("expected messages array, got: %v", result)
	}

	msg := messages[0].(map[string]any)
	content := msg["content"].(map[string]any)
	text := content["text"].(string)

	if !strings.Contains(text, "Test User") {
		t.Errorf("expected greeting to contain 'Test User', got: %s", text)
	}
}

func TestStdioProxyResourcesList(t *testing.T) {
	port := getFreePort(t)
	cleanup := startProxy(t, port)
	defer cleanup()

	resp := mcpRequest(t, port, "resources/list", nil)

	if errVal, ok := resp["error"]; ok {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got: %v", resp)
	}

	resources, ok := result["resources"].([]any)
	if !ok {
		t.Fatalf("expected resources array, got: %v", result)
	}

	if len(resources) == 0 {
		t.Error("expected at least one resource")
	}
}

func TestStdioProxyResourceRead(t *testing.T) {
	port := getFreePort(t)
	cleanup := startProxy(t, port)
	defer cleanup()

	resp := mcpRequest(t, port, "resources/read", map[string]any{
		"uri": "test://hello",
	})

	if errVal, ok := resp["error"]; ok {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got: %v", resp)
	}

	contents, ok := result["contents"].([]any)
	if !ok || len(contents) == 0 {
		t.Fatalf("expected contents array, got: %v", result)
	}

	content := contents[0].(map[string]any)
	text := content["text"].(string)

	if !strings.Contains(text, "Hello from test resource") {
		t.Errorf("expected resource text to contain 'Hello from test resource', got: %s", text)
	}
}

func TestStdioProxyConcurrentRequests(t *testing.T) {
	port := getFreePort(t)
	cleanup := startProxy(t, port)
	defer cleanup()

	// Send multiple concurrent requests
	const numRequests = 10
	results := make(chan error, numRequests)

	for i := range numRequests {
		go func(i int) {
			resp := mcpRequest(t, port, "tools/call", map[string]any{
				"name":      "echo",
				"arguments": map[string]any{"message": fmt.Sprintf("request-%d", i)},
			})

			if _, ok := resp["error"]; ok {
				results <- fmt.Errorf("request %d failed: %v", i, resp["error"])
				return
			}

			result := resp["result"].(map[string]any)
			content := result["content"].([]any)
			firstContent := content[0].(map[string]any)
			text := firstContent["text"].(string)

			expected := fmt.Sprintf("request-%d", i)
			if text != expected {
				results <- fmt.Errorf("request %d: got %q, want %q", i, text, expected)
				return
			}

			results <- nil
		}(i)
	}

	// Collect results
	for range numRequests {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
}
