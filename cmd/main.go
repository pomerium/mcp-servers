package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

var programName = filepath.Base(os.Args[0])

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return printUsage()
	}

	cmd, cmdArgs := args[0], args[1:]

	switch cmd {
	case "serve":
		return serveCommand(ctx, cmdArgs)
	case "stdio-proxy":
		return stdioProxyCommand(ctx, cmdArgs)
	case "help", "-h", "--help":
		return printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		return printUsage()
	}
}

func printUsage() error {
	fmt.Fprintf(os.Stderr, `Usage: %s <command> [options]

Commands:
  serve         Start the MCP server
  stdio-proxy   Proxy HTTP requests to a stdio MCP server

Use "%s <command> -h" for more information about a command.
`, programName, programName)
	return nil
}
