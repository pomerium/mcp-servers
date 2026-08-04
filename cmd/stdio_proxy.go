package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/pomerium/mcp-servers/httputil"
	"github.com/pomerium/mcp-servers/stdioproxy"
)

func stdioProxyCommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("stdio-proxy", flag.ExitOnError)

	addr := fs.String("addr", "", "HTTP bind address (default from ADDR env or :8080)")
	logLevel := fs.String("log-level", "", "Log level: debug, info, warn, error (default from LOG_LEVEL env or info)")
	workDir := fs.String("work-dir", "", "Working directory for subprocess")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: %s stdio-proxy [options] -- <command> [args...]

Proxy HTTP streaming requests to a stdio MCP server.

Options:
`, programName)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Environment variables:
  ADDR       HTTP bind address (default :8080)
  LOG_LEVEL  Log level: debug, info, warn, error (default info)

Examples:
  # Proxy requests to a Node.js MCP server
  %s stdio-proxy -- node /path/to/server.js

  # Proxy with custom port and debug logging
  %s stdio-proxy -addr :9090 -log-level debug -- python -m my_mcp_server

  # Using environment variables
  ADDR=:9090 LOG_LEVEL=debug %s stdio-proxy -- ./my-server
`, programName, programName, programName)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Remaining args are the subprocess command
	subArgs := fs.Args()
	if len(subArgs) == 0 {
		fs.Usage()
		return fmt.Errorf("missing subprocess command")
	}

	// Resolve bind address
	bindAddr := *addr
	if bindAddr == "" {
		bindAddr = os.Getenv("ADDR")
	}
	if bindAddr == "" {
		bindAddr = ":8080"
	}

	// Resolve log level
	level := *logLevel
	if level == "" {
		level = os.Getenv("LOG_LEVEL")
	}
	if level == "" {
		level = "info"
	}

	// Setup logger (JSON for daemon, text for terminal)
	logger := stdioproxy.SetupLogger(stdioproxy.ParseLogLevel(level))
	slog.SetDefault(logger)

	logger.Info("starting stdio-proxy",
		"addr", bindAddr,
		"command", subArgs[0],
		"args", subArgs[1:],
	)

	// Create process manager
	pm := stdioproxy.NewProcessManager(stdioproxy.ProcessManagerConfig{
		Command: subArgs[0],
		Args:    subArgs[1:],
		WorkDir: *workDir,
		Logger:  logger,
	})

	// Start the subprocess
	if err := pm.Start(ctx); err != nil {
		return fmt.Errorf("start subprocess: %w", err)
	}

	// Ensure cleanup on shutdown
	defer func() {
		if err := pm.Stop(); err != nil {
			logger.Warn("error stopping subprocess", "error", err)
		}
	}()

	// Create proxy server
	proxyServer := stdioproxy.NewProxyServer(pm, logger)

	// Create HTTP handler
	handler, err := stdioproxy.NewHTTPHandler(pm, proxyServer, logger)
	if err != nil {
		return fmt.Errorf("create HTTP handler: %w", err)
	}

	// Start HTTP server (blocks until context is cancelled)
	return httputil.ListenAndServe(ctx, bindAddr, handler)
}
