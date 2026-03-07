package pomerium_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	pomeriumpkg "github.com/pomerium/mcp-servers/pomerium"
	"github.com/pomerium/sdk-go"
	sdkproto "github.com/pomerium/sdk-go/proto/pomerium"
)

// TestE2E starts a real Pomerium core instance via testcontainers and
// exercises the MCP server end-to-end using the MCP client SDK.
func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := t.Context()

	apiURL, sharedSecretB64 := startTestPomeriumCore(ctx, t)
	t.Logf("Pomerium API URL: %s", apiURL)

	// Create the MCP server pointing at the real Pomerium instance
	env := map[string]string{
		"API_URL":   apiURL,
		"API_TOKEN": sharedSecretB64,
	}
	mcpServer, err := pomeriumpkg.NewServer(ctx, env)
	if err != nil {
		t.Fatal(err)
	}

	// Connect MCP client
	ct, st := mcp.NewInMemoryTransports()
	go mcpServer.Run(ctx, st)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "e2e-test", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}

	// List available tools
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Got %d tools", len(tools.Tools))
	if len(tools.Tools) != 23 {
		t.Errorf("expected 23 tools, got %d", len(tools.Tools))
	}

	// Test: get_settings (read-only, should work)
	t.Run("get_settings", func(t *testing.T) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "get_settings",
			Arguments: json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			textContent := result.Content[0].(*mcp.TextContent)
			t.Fatalf("get_settings returned error: %s", textContent.Text)
		}
		textContent := result.Content[0].(*mcp.TextContent)
		t.Logf("get_settings response: %s", truncate(textContent.Text, 500))
	})

	// Test: list_routes (should return empty list initially)
	t.Run("list_routes", func(t *testing.T) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "list_routes",
			Arguments: json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			textContent := result.Content[0].(*mcp.TextContent)
			t.Fatalf("list_routes returned error: %s", textContent.Text)
		}
		textContent := result.Content[0].(*mcp.TextContent)
		t.Logf("list_routes response: %s", truncate(textContent.Text, 500))
	})

	// Test: create_route, get_route, delete_route (full CRUD cycle)
	t.Run("route_crud", func(t *testing.T) {
		// Create
		createResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "create_route",
			Arguments: json.RawMessage(`{
				"route": {
					"name": "test-route",
					"from": "https://test.example.com",
					"to": ["https://backend.example.com"]
				}
			}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if createResult.IsError {
			textContent := createResult.Content[0].(*mcp.TextContent)
			t.Fatalf("create_route error: %s", textContent.Text)
		}

		textContent := createResult.Content[0].(*mcp.TextContent)
		t.Logf("create_route response: %s", truncate(textContent.Text, 500))

		// Extract route ID from response
		var createResp map[string]any
		if err := json.Unmarshal([]byte(textContent.Text), &createResp); err != nil {
			t.Fatalf("parsing create response: %v", err)
		}
		route, ok := createResp["route"].(map[string]any)
		if !ok {
			t.Fatal("expected route in create response")
		}
		routeID, ok := route["id"].(string)
		if !ok || routeID == "" {
			t.Fatal("expected route id in create response")
		}
		t.Logf("Created route with ID: %s", routeID)

		// Get
		getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "get_route",
			Arguments: json.RawMessage(fmt.Sprintf(`{"id": %q}`, routeID)),
		})
		if err != nil {
			t.Fatal(err)
		}
		if getResult.IsError {
			textContent := getResult.Content[0].(*mcp.TextContent)
			t.Fatalf("get_route error: %s", textContent.Text)
		}
		textContent = getResult.Content[0].(*mcp.TextContent)
		t.Logf("get_route response: %s", truncate(textContent.Text, 500))

		// Delete
		deleteResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "delete_route",
			Arguments: json.RawMessage(fmt.Sprintf(`{"id": %q}`, routeID)),
		})
		if err != nil {
			t.Fatal(err)
		}
		if deleteResult.IsError {
			textContent := deleteResult.Content[0].(*mcp.TextContent)
			t.Fatalf("delete_route error: %s", textContent.Text)
		}
		t.Log("Route deleted successfully")
	})
}

func startTestPomeriumCore(ctx context.Context, t *testing.T) (apiURL, sharedSecretB64 string) {
	t.Helper()

	sharedSecret := make([]byte, 32)
	for i := range sharedSecret {
		sharedSecret[i] = byte(i + 1)
	}
	sharedSecretB64 = base64.StdEncoding.EncodeToString(sharedSecret)

	container, err := testcontainers.Run(ctx, "pomerium/pomerium:main",
		testcontainers.WithAlwaysPull(),
		testcontainers.WithExposedPorts("5443/tcp"),
		testcontainers.WithLogConsumers(testLogConsumer{t}),
		testcontainers.WithEnv(map[string]string{
			"GRPC_ADDRESS":  "0.0.0.0:5443",
			"SHARED_SECRET": sharedSecretB64,
		}),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5443/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatal(err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mappedPort, err := container.MappedPort(ctx, "5443/tcp")
	if err != nil {
		t.Fatal(err)
	}

	apiURL = fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	// Wait for Pomerium to be ready
	pollInterval := time.NewTicker(100 * time.Millisecond)
	defer pollInterval.Stop()

	deadline := time.After(60 * time.Second)
	for {
		_, err = sdk.NewClient(
			sdk.WithAPIToken(sharedSecretB64),
			sdk.WithURL(apiURL),
		).GetServerInfo(ctx, connect.NewRequest(&sdkproto.GetServerInfoRequest{}))
		if err == nil {
			break
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for pomerium: %v", err)
		case <-pollInterval.C:
		}
	}

	return apiURL, sharedSecretB64
}

type testLogConsumer struct {
	testing.TB
}

func (lc testLogConsumer) Accept(l testcontainers.Log) {
	lc.Log(strings.TrimSpace(string(l.Content)))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
