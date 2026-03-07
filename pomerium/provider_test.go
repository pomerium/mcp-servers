package pomerium_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	pomeriumpkg "github.com/pomerium/mcp-servers/pomerium"
	sdkproto "github.com/pomerium/sdk-go/proto/pomerium"
)

func TestRegisterToolsCount(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	caller := pomeriumpkg.NewDynamicCaller("http://localhost", http.DefaultTransport)

	pomeriumpkg.RegisterTools(s, caller, sdkproto.File_config_proto)

	// List tools by connecting as a client
	ctx := t.Context()
	ct, st := mcp.NewInMemoryTransports()
	go s.Run(ctx, st)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	// We expect 23 tools (24 methods minus GetServerInfo)
	if len(tools.Tools) != 23 {
		t.Errorf("expected 23 tools, got %d", len(tools.Tools))
		for _, tool := range tools.Tools {
			t.Logf("  tool: %s", tool.Name)
		}
	}

	// Verify a few specific tools exist with correct annotations
	toolMap := make(map[string]*mcp.Tool)
	for _, tool := range tools.Tools {
		toolMap[tool.Name] = tool
	}

	// get_route should be read-only
	if tool, ok := toolMap["get_route"]; ok {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Error("get_route should have ReadOnlyHint=true")
		}
	} else {
		t.Error("get_route tool not found")
	}

	// delete_route should be destructive
	if tool, ok := toolMap["delete_route"]; ok {
		if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
			t.Error("delete_route should have DestructiveHint=true")
		}
	} else {
		t.Error("delete_route tool not found")
	}

	// create_route should have output schema
	if tool, ok := toolMap["create_route"]; ok {
		if tool.OutputSchema == nil {
			t.Error("create_route should have OutputSchema")
		}
	} else {
		t.Error("create_route tool not found")
	}
}

func TestDynamicCallerCall(t *testing.T) {
	// Set up a mock Connect server that echoes back a known response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Connect protocol headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Connect-Protocol-Version") != "1" {
			t.Errorf("expected Connect-Protocol-Version: 1, got %s", r.Header.Get("Connect-Protocol-Version"))
		}

		// Read and verify input
		body, _ := io.ReadAll(r.Body)
		var input map[string]any
		json.Unmarshal(body, &input)

		// Return a mock response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"route": map[string]any{
				"id":   input["id"],
				"name": "test-route",
			},
		})
	}))
	defer mockServer.Close()

	caller := pomeriumpkg.NewDynamicCaller(mockServer.URL, http.DefaultTransport)

	// Get the GetRoute method descriptor
	svc := sdkproto.File_config_proto.Services().ByName("ConfigService")
	method := svc.Methods().ByName("GetRoute")

	ctx := t.Context()
	resp, err := caller.Call(ctx, method, json.RawMessage(`{"id":"abc"}`))
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatal(err)
	}

	route, ok := result["route"].(map[string]any)
	if !ok {
		t.Fatal("expected route in response")
	}
	if route["id"] != "abc" {
		t.Errorf("expected id=abc, got %v", route["id"])
	}
	if route["name"] != "test-route" {
		t.Errorf("expected name=test-route, got %v", route["name"])
	}
}

func TestEndToEndToolCall(t *testing.T) {
	// Set up a mock Connect server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/pomerium.config.ConfigService/GetServerInfo":
			json.NewEncoder(w).Encode(map[string]any{
				"serverType": "SERVER_TYPE_CORE",
			})
		case "/pomerium.config.ConfigService/ListRoutes":
			json.NewEncoder(w).Encode(map[string]any{
				"routes": []map[string]any{
					{"id": "r1", "name": "route-1", "from": "https://app.example.com"},
					{"id": "r2", "name": "route-2", "from": "https://api.example.com"},
				},
			})
		default:
			http.Error(w, `{"code":"unimplemented","message":"not implemented"}`, http.StatusNotImplemented)
		}
	}))
	defer mockServer.Close()

	// Create the MCP server with the mock backend
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	authRT := pomeriumpkg.NewAuthTransport(
		pomeriumpkg.AuthConfig{APIToken: "test-token", BaseURL: mockServer.URL},
		http.DefaultTransport,
	)
	caller := pomeriumpkg.NewDynamicCaller(mockServer.URL, authRT)
	pomeriumpkg.RegisterTools(mcpServer, caller, sdkproto.File_config_proto)

	// Connect MCP client to server
	ctx := t.Context()
	ct, st := mcp.NewInMemoryTransports()
	go mcpServer.Run(ctx, st)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Call the list_routes tool via MCP
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_routes",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	// Verify we got the expected routes back
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(textContent.Text), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	routes, ok := response["routes"].([]any)
	if !ok {
		t.Fatal("expected routes array in response")
	}
	if len(routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(routes))
	}
}
