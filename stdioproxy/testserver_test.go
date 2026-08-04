package stdioproxy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestServer creates an in-process MCP test server and returns a transport
// that can be used by ProcessManager to connect to it.
// The server runs in a goroutine and is automatically stopped when the test completes.
func newTestServer(t *testing.T) mcp.Transport {
	t.Helper()

	// Create connected in-memory transports
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Create the test server
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "test-server",
			Version: "1.0.0",
		},
		nil,
	)

	// Echo tool - returns the input message
	type echoArgs struct {
		Message string `json:"message" jsonschema:"The message to echo back"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Echo a message back",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: args.Message},
			},
		}, nil, nil
	})

	// Add tool - adds two numbers
	type addArgs struct {
		A int `json:"a" jsonschema:"First number"`
		B int `json:"b" jsonschema:"Second number"`
	}
	type addResult struct {
		Result int `json:"result"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add",
		Description: "Add two numbers",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args addArgs) (*mcp.CallToolResult, addResult, error) {
		return nil, addResult{Result: args.A + args.B}, nil
	})

	// Slow tool - for testing timeouts
	type slowArgs struct {
		DelayMS int `json:"delay_ms" jsonschema:"Delay in milliseconds before responding"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "slow",
		Description: "Responds after a delay",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args slowArgs) (*mcp.CallToolResult, any, error) {
		select {
		case <-time.After(time.Duration(args.DelayMS) * time.Millisecond):
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "done"},
				},
			}, nil, nil
		case <-ctx.Done():
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "cancelled"},
				},
				IsError: true,
			}, nil, nil
		}
	})

	// Error tool - always returns an error
	mcp.AddTool(server, &mcp.Tool{
		Name:        "error",
		Description: "Always returns an error",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "intentional error"},
			},
			IsError: true,
		}, nil, nil
	})

	// Exit tool - for testing, returns an error to simulate failure
	type exitArgs struct {
		Code int `json:"code" jsonschema:"Exit code"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "exit",
		Description: "Simulate server exit (returns error in test mode)",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ exitArgs) (*mcp.CallToolResult, any, error) {
		// In test mode, we can't actually exit, so return an error
		return nil, nil, errors.New("simulated exit")
	})

	// Add a test prompt
	server.AddPrompt(&mcp.Prompt{
		Name:        "greeting",
		Description: "A simple greeting prompt",
		Arguments: []*mcp.PromptArgument{
			{Name: "name", Description: "Name to greet", Required: true},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		name := "World"
		if n, ok := req.Params.Arguments["name"]; ok && n != "" {
			name = n
		}
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{
						Text: "Hello, " + name + "!",
					},
				},
			},
		}, nil
	})

	// Add a test resource
	server.AddResource(&mcp.Resource{
		URI:         "test://hello",
		Name:        "Hello Resource",
		Description: "A simple test resource",
		MIMEType:    "text/plain",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      "test://hello",
					MIMEType: "text/plain",
					Text:     "Hello from test resource!",
				},
			},
		}, nil
	})

	// Start the server in a goroutine
	// The server will automatically connect using the serverTransport
	go func() {
		_ = server.Run(t.Context(), serverTransport)
	}()

	return clientTransport
}
