package stdioproxy_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pomerium/mcp-servers/stdioproxy"
)

// newBidirectionalTestServer creates a test server that supports bidirectional features.
func newBidirectionalTestServer(t *testing.T) mcp.Transport {
	t.Helper()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "bidirectional-test-server",
			Version: "1.0.0",
		},
		nil,
	)

	// Tool that triggers sampling (CreateMessage request to client)
	type samplingArgs struct {
		Prompt string `json:"prompt" jsonschema:"The prompt to send"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "request_sampling",
		Description: "Request LLM sampling from the client",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args samplingArgs) (*mcp.CallToolResult, any, error) {
		session := req.Session
		result, err := session.CreateMessage(ctx, &mcp.CreateMessageParams{
			Messages: []*mcp.SamplingMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{
						Text: args.Prompt,
					},
				},
			},
			MaxTokens: 100,
		})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "sampling error: " + err.Error()},
				},
				IsError: true,
			}, nil, nil
		}
		// Extract text from the response
		text := ""
		if tc, ok := result.Content.(*mcp.TextContent); ok {
			text = tc.Text
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "sampling result: " + text},
			},
		}, nil, nil
	})

	// Tool that triggers elicitation (request user input from client)
	type elicitArgs struct {
		Message string `json:"message" jsonschema:"Message to show user"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "request_elicitation",
		Description: "Request user input from the client",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args elicitArgs) (*mcp.CallToolResult, any, error) {
		session := req.Session
		result, err := session.Elicit(ctx, &mcp.ElicitParams{
			Message: args.Message,
			RequestedSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"answer": map[string]any{
						"type":        "string",
						"description": "User's answer",
					},
				},
				"required": []string{"answer"},
			},
		})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "elicitation error: " + err.Error()},
				},
				IsError: true,
			}, nil, nil
		}
		// Check action
		if result.Action == "cancel" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "elicitation cancelled"},
				},
			}, nil, nil
		}
		// Extract answer from content
		answer := ""
		if result.Content != nil {
			if ans, ok := result.Content["answer"].(string); ok {
				answer = ans
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "elicitation result: " + answer},
			},
		}, nil, nil
	})

	// Tool that sends a logging message
	type logArgs struct {
		Level   string `json:"level" jsonschema:"Log level (debug, info, warning, error)"`
		Message string `json:"message" jsonschema:"Log message"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_log",
		Description: "Send a logging message to the client",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args logArgs) (*mcp.CallToolResult, any, error) {
		session := req.Session
		err := session.Log(ctx, &mcp.LoggingMessageParams{
			Level:  mcp.LoggingLevel(args.Level),
			Logger: "test-server",
			Data:   args.Message,
		})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "logging error: " + err.Error()},
				},
				IsError: true,
			}, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "log sent"},
			},
		}, nil, nil
	})

	// Simple echo tool for basic tests
	type echoArgs struct {
		Message string `json:"message"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Echo a message",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: args.Message},
			},
		}, nil, nil
	})

	// Start server
	go func() {
		_ = server.Run(t.Context(), serverTransport)
	}()

	return clientTransport
}

func TestLoggingNotification(t *testing.T) {
	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var loggingCalled atomic.Int32
	var loggingMessage string
	var loggingMu sync.Mutex

	transport := newBidirectionalTestServer(t)

	pm := stdioproxy.NewProcessManager(stdioproxy.ProcessManagerConfig{
		Transport: transport,
		Logger:    logger,
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			loggingCalled.Add(1)
			loggingMu.Lock()
			if data, ok := req.Params.Data.(string); ok {
				loggingMessage = data
			}
			loggingMu.Unlock()
		},
	})

	if err := pm.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer pm.Stop()

	session, err := pm.GetSession()
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	// Set logging level so server will send logs
	if err := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "debug"}); err != nil {
		t.Fatalf("failed to set logging level: %v", err)
	}

	// Call the send_log tool which triggers a logging notification
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "send_log",
		Arguments: map[string]any{
			"level":   "info",
			"message": "test log message",
		},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	if loggingCalled.Load() == 0 {
		t.Error("logging handler was not called")
	}

	loggingMu.Lock()
	if loggingMessage != "test log message" {
		t.Errorf("logging message = %q, want %q", loggingMessage, "test log message")
	}
	loggingMu.Unlock()
}

func TestSamplingHandler(t *testing.T) {
	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var samplingCalled atomic.Int32
	var samplingPrompt string
	var samplingMu sync.Mutex

	transport := newBidirectionalTestServer(t)

	pm := stdioproxy.NewProcessManager(stdioproxy.ProcessManagerConfig{
		Transport: transport,
		Logger:    logger,
		CreateMessageHandler: func(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			samplingCalled.Add(1)
			samplingMu.Lock()
			// Extract prompt from messages
			if len(req.Params.Messages) > 0 {
				if tc, ok := req.Params.Messages[0].Content.(*mcp.TextContent); ok {
					samplingPrompt = tc.Text
				}
			}
			samplingMu.Unlock()

			// Return a mock response
			return &mcp.CreateMessageResult{
				Role:    "assistant",
				Content: &mcp.TextContent{Text: "mock LLM response"},
				Model:   "test-model",
			}, nil
		},
	})

	if err := pm.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer pm.Stop()

	session, err := pm.GetSession()
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	// Call the request_sampling tool which triggers a CreateMessage request
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "request_sampling",
		Arguments: map[string]any{
			"prompt": "What is 2+2?",
		},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	if samplingCalled.Load() == 0 {
		t.Error("sampling handler was not called")
	}

	samplingMu.Lock()
	if samplingPrompt != "What is 2+2?" {
		t.Errorf("sampling prompt = %q, want %q", samplingPrompt, "What is 2+2?")
	}
	samplingMu.Unlock()

	// Verify the response contains our mock response
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		if tc.Text != "sampling result: mock LLM response" {
			t.Errorf("result text = %q, want %q", tc.Text, "sampling result: mock LLM response")
		}
	}
}

func TestElicitationHandler(t *testing.T) {
	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var elicitCalled atomic.Int32
	var elicitMessage string
	var elicitMu sync.Mutex

	transport := newBidirectionalTestServer(t)

	pm := stdioproxy.NewProcessManager(stdioproxy.ProcessManagerConfig{
		Transport: transport,
		Logger:    logger,
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			elicitCalled.Add(1)
			elicitMu.Lock()
			elicitMessage = req.Params.Message
			elicitMu.Unlock()

			// Return a mock user response
			return &mcp.ElicitResult{
				Action: "accept",
				Content: map[string]any{
					"answer": "user's answer",
				},
			}, nil
		},
	})

	if err := pm.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer pm.Stop()

	session, err := pm.GetSession()
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	// Call the request_elicitation tool which triggers an Elicit request
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "request_elicitation",
		Arguments: map[string]any{
			"message": "Please provide input",
		},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}

	if elicitCalled.Load() == 0 {
		t.Error("elicitation handler was not called")
	}

	elicitMu.Lock()
	if elicitMessage != "Please provide input" {
		t.Errorf("elicit message = %q, want %q", elicitMessage, "Please provide input")
	}
	elicitMu.Unlock()

	// Verify the response contains the user's answer
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		if tc.Text != "elicitation result: user's answer" {
			t.Errorf("result text = %q, want %q", tc.Text, "elicitation result: user's answer")
		}
	}
}

func TestSamplingWithoutHandler(t *testing.T) {
	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	transport := newBidirectionalTestServer(t)

	// Don't set a CreateMessageHandler - should get an error
	pm := stdioproxy.NewProcessManager(stdioproxy.ProcessManagerConfig{
		Transport: transport,
		Logger:    logger,
	})

	if err := pm.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer pm.Stop()

	session, err := pm.GetSession()
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	// Call the request_sampling tool - should fail because no handler
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "request_sampling",
		Arguments: map[string]any{
			"prompt": "test",
		},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}

	// The tool should return an error because sampling is not supported
	if !result.IsError {
		t.Error("expected error when sampling without handler")
	}
}

func TestElicitationWithoutHandler(t *testing.T) {
	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	transport := newBidirectionalTestServer(t)

	// Don't set an ElicitationHandler - should get an error
	pm := stdioproxy.NewProcessManager(stdioproxy.ProcessManagerConfig{
		Transport: transport,
		Logger:    logger,
	})

	if err := pm.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer pm.Stop()

	session, err := pm.GetSession()
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	// Call the request_elicitation tool - should fail because no handler
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "request_elicitation",
		Arguments: map[string]any{
			"message": "test",
		},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}

	// The tool should return an error because elicitation is not supported
	if !result.IsError {
		t.Error("expected error when elicitation without handler")
	}
}
