package stdioproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// proxyImplementation is the shared MCP implementation info for the proxy.
var proxyImplementation = &mcp.Implementation{
	Name:    "stdio-proxy",
	Version: "1.0.0",
}

// ProxyStats tracks statistics about proxied requests.
type ProxyStats struct {
	RequestCount  atomic.Int64
	ResponseCount atomic.Int64
	ErrorCount    atomic.Int64
}

// ProxyServer creates an MCP server that proxies all requests to a subprocess.
type ProxyServer struct {
	pm     *ProcessManager
	logger *slog.Logger
	stats  *ProxyStats
}

// NewProxyServer creates a new ProxyServer that forwards requests to the given ProcessManager.
func NewProxyServer(pm *ProcessManager, logger *slog.Logger) *ProxyServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProxyServer{
		pm:     pm,
		logger: logger,
		stats:  &ProxyStats{},
	}
}

// Stats returns the current proxy statistics.
func (ps *ProxyServer) Stats() *ProxyStats {
	return ps.stats
}

// BuildMCPServer creates an MCP server configured to proxy requests to the subprocess.
// It discovers capabilities from the subprocess and registers forwarding handlers.
func (ps *ProxyServer) BuildMCPServer(ctx context.Context) (*mcp.Server, error) {
	session, err := ps.pm.GetSession()
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Create the proxy MCP server
	mcpServer := mcp.NewServer(proxyImplementation, nil)

	// Discover and register tools from the subprocess
	if err := ps.registerTools(ctx, mcpServer, session); err != nil {
		ps.logger.Warn("failed to register tools", "error", err)
	}

	// Discover and register prompts from the subprocess
	if err := ps.registerPrompts(ctx, mcpServer, session); err != nil {
		ps.logger.Warn("failed to register prompts", "error", err)
	}

	// Discover and register resources from the subprocess
	if err := ps.registerResources(ctx, mcpServer, session); err != nil {
		ps.logger.Warn("failed to register resources", "error", err)
	}

	// Discover and register resource templates from the subprocess
	if err := ps.registerResourceTemplates(ctx, mcpServer, session); err != nil {
		ps.logger.Warn("failed to register resource templates", "error", err)
	}

	return mcpServer, nil
}

// registerTools discovers tools from the subprocess and registers forwarding handlers.
func (ps *ProxyServer) registerTools(ctx context.Context, server *mcp.Server, session *mcp.ClientSession) error {
	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}

	ps.logger.Info("discovered tools from subprocess", "count", len(result.Tools))

	for _, tool := range result.Tools {
		// Capture tool for closure
		t := tool
		ps.logger.Debug("registering tool", "name", t.Name)

		// Register with the low-level handler since we're forwarding raw requests
		server.AddTool(t, ps.createToolHandler(t.Name))
	}

	return nil
}

// createToolHandler creates a handler that forwards tool calls to the subprocess.
func (ps *ProxyServer) createToolHandler(toolName string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		ps.stats.RequestCount.Add(1)

		ps.logger.Info("proxying tool call",
			"tool", toolName,
		)

		// Log request arguments at debug level
		if ps.logger.Enabled(ctx, slog.LevelDebug) {
			argsJSON, _ := json.Marshal(req.Params.Arguments)
			ps.logger.Debug("tool call request", "arguments", string(argsJSON))
		}

		session, err := ps.pm.GetSession()
		if err != nil {
			ps.stats.ErrorCount.Add(1)
			ps.logger.Error("failed to get session for tool call",
				"tool", toolName,
				"error", err,
			)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("proxy error: %v", err)},
				},
				IsError: true,
			}, nil
		}

		// Forward the call to the subprocess
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      toolName,
			Arguments: req.Params.Arguments,
		})

		duration := time.Since(start)

		if err != nil {
			ps.stats.ErrorCount.Add(1)
			ps.logger.Error("tool call failed",
				"tool", toolName,
				"duration", duration,
				"error", err,
			)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("subprocess error: %v", err)},
				},
				IsError: true,
			}, nil
		}

		ps.stats.ResponseCount.Add(1)

		ps.logger.Info("tool call completed",
			"tool", toolName,
			"duration", duration,
			"isError", result.IsError,
		)

		// Log full response at debug level
		if ps.logger.Enabled(ctx, slog.LevelDebug) {
			respJSON, _ := json.Marshal(result)
			ps.logger.Debug("tool call response", "body", string(respJSON))
		}

		return result, nil
	}
}

// registerPrompts discovers prompts from the subprocess and registers forwarding handlers.
func (ps *ProxyServer) registerPrompts(ctx context.Context, server *mcp.Server, session *mcp.ClientSession) error {
	result, err := session.ListPrompts(ctx, &mcp.ListPromptsParams{})
	if err != nil {
		return fmt.Errorf("list prompts: %w", err)
	}

	ps.logger.Info("discovered prompts from subprocess", "count", len(result.Prompts))

	for _, prompt := range result.Prompts {
		p := prompt
		ps.logger.Debug("registering prompt", "name", p.Name)

		server.AddPrompt(p, ps.createPromptHandler(p.Name))
	}

	return nil
}

// createPromptHandler creates a handler that forwards prompt requests to the subprocess.
func (ps *ProxyServer) createPromptHandler(promptName string) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		start := time.Now()
		ps.stats.RequestCount.Add(1)

		ps.logger.Info("proxying prompt request",
			"prompt", promptName,
		)

		session, err := ps.pm.GetSession()
		if err != nil {
			ps.stats.ErrorCount.Add(1)
			return nil, fmt.Errorf("get session: %w", err)
		}

		result, err := session.GetPrompt(ctx, &mcp.GetPromptParams{
			Name:      promptName,
			Arguments: req.Params.Arguments,
		})

		duration := time.Since(start)

		if err != nil {
			ps.stats.ErrorCount.Add(1)
			ps.logger.Error("prompt request failed",
				"prompt", promptName,
				"duration", duration,
				"error", err,
			)
			return nil, err
		}

		ps.stats.ResponseCount.Add(1)
		ps.logger.Info("prompt request completed",
			"prompt", promptName,
			"duration", duration,
		)

		return result, nil
	}
}

// registerResources discovers resources from the subprocess and registers forwarding handlers.
func (ps *ProxyServer) registerResources(ctx context.Context, server *mcp.Server, session *mcp.ClientSession) error {
	result, err := session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		return fmt.Errorf("list resources: %w", err)
	}

	ps.logger.Info("discovered resources from subprocess", "count", len(result.Resources))

	for _, resource := range result.Resources {
		r := resource
		ps.logger.Debug("registering resource", "uri", r.URI)

		server.AddResource(r, ps.createResourceHandler(r.URI))
	}

	return nil
}

// createResourceHandler creates a handler that forwards resource requests to the subprocess.
func (ps *ProxyServer) createResourceHandler(uri string) mcp.ResourceHandler {
	return func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		start := time.Now()
		ps.stats.RequestCount.Add(1)

		ps.logger.Info("proxying resource request",
			"uri", uri,
		)

		session, err := ps.pm.GetSession()
		if err != nil {
			ps.stats.ErrorCount.Add(1)
			return nil, fmt.Errorf("get session: %w", err)
		}

		result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
			URI: uri,
		})

		duration := time.Since(start)

		if err != nil {
			ps.stats.ErrorCount.Add(1)
			ps.logger.Error("resource request failed",
				"uri", uri,
				"duration", duration,
				"error", err,
			)
			return nil, err
		}

		ps.stats.ResponseCount.Add(1)
		ps.logger.Info("resource request completed",
			"uri", uri,
			"duration", duration,
		)

		return result, nil
	}
}

// registerResourceTemplates discovers resource templates from the subprocess and registers forwarding handlers.
func (ps *ProxyServer) registerResourceTemplates(ctx context.Context, server *mcp.Server, session *mcp.ClientSession) error {
	result, err := session.ListResourceTemplates(ctx, &mcp.ListResourceTemplatesParams{})
	if err != nil {
		return fmt.Errorf("list resource templates: %w", err)
	}

	ps.logger.Info("discovered resource templates from subprocess", "count", len(result.ResourceTemplates))

	for _, tmpl := range result.ResourceTemplates {
		t := tmpl
		ps.logger.Debug("registering resource template", "uriTemplate", t.URITemplate)

		server.AddResourceTemplate(t, ps.createResourceTemplateHandler(t.URITemplate))
	}

	return nil
}

// createResourceTemplateHandler creates a handler that forwards resource template requests to the subprocess.
func (ps *ProxyServer) createResourceTemplateHandler(uriTemplate string) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		start := time.Now()
		ps.stats.RequestCount.Add(1)

		// The actual URI to read comes from the request
		uri := req.Params.URI

		ps.logger.Info("proxying resource template request",
			"template", uriTemplate,
			"uri", uri,
		)

		session, err := ps.pm.GetSession()
		if err != nil {
			ps.stats.ErrorCount.Add(1)
			return nil, fmt.Errorf("get session: %w", err)
		}

		result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
			URI: uri,
		})

		duration := time.Since(start)

		if err != nil {
			ps.stats.ErrorCount.Add(1)
			ps.logger.Error("resource template request failed",
				"template", uriTemplate,
				"uri", uri,
				"duration", duration,
				"error", err,
			)
			return nil, err
		}

		ps.stats.ResponseCount.Add(1)
		ps.logger.Info("resource template request completed",
			"template", uriTemplate,
			"uri", uri,
			"duration", duration,
		)

		return result, nil
	}
}

// HTTPHandler wraps the MCP server in an HTTP handler that checks process health.
type HTTPHandler struct {
	pm          *ProcessManager
	mcpHandler  http.Handler
	logger      *slog.Logger
	proxyServer *ProxyServer
}

// NewHTTPHandler creates an HTTP handler that serves MCP requests.
// It wraps the StreamableHTTPHandler with health checking and logging.
func NewHTTPHandler(pm *ProcessManager, proxyServer *ProxyServer, logger *slog.Logger) (*HTTPHandler, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Build the MCP server
	mcpServer, err := proxyServer.BuildMCPServer(context.Background())
	if err != nil {
		return nil, fmt.Errorf("build MCP server: %w", err)
	}

	// Create the streamable HTTP handler
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server {
			return mcpServer
		},
		&mcp.StreamableHTTPOptions{
			Stateless: true,
		},
	)

	return &HTTPHandler{
		pm:          pm,
		mcpHandler:  mcpHandler,
		logger:      logger,
		proxyServer: proxyServer,
	}, nil
}

// ServeHTTP implements http.Handler.
// It checks if the subprocess is healthy before forwarding requests.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check subprocess health
	state := h.pm.State()
	if state != StateRunning {
		h.logger.Error("subprocess not running",
			"state", state.String(),
			"error", h.pm.Error(),
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		errMsg := "subprocess not available"
		if err := h.pm.Error(); err != nil {
			errMsg = err.Error()
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","error":{"code":-32000,"message":%q},"id":null}`, errMsg)
		return
	}

	// Log the request
	h.logger.Info("http request",
		"method", r.Method,
		"path", r.URL.Path,
		"remote", r.RemoteAddr,
	)

	// Forward to the MCP handler
	h.mcpHandler.ServeHTTP(w, r)
}
