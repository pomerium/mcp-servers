package stdioproxy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProcessState represents the current state of the subprocess.
type ProcessState int

const (
	// StateStopped indicates the process has not been started.
	StateStopped ProcessState = iota
	// StateRunning indicates the process is running normally.
	StateRunning
	// StateCrashed indicates the process terminated unexpectedly.
	StateCrashed
)

func (s ProcessState) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateRunning:
		return "running"
	case StateCrashed:
		return "crashed"
	default:
		return "unknown"
	}
}

// ProcessManager manages the lifecycle of a stdio MCP server subprocess.
// It provides thread-safe access to the MCP client session connected to the subprocess.
type ProcessManager struct {
	cmdPath   string
	cmdArgs   []string
	env       []string
	workDir   string
	transport mcp.Transport // optional pre-configured transport (for testing)

	// Notification handlers
	toolListChangedHandler      func(context.Context, *mcp.ToolListChangedRequest)
	promptListChangedHandler    func(context.Context, *mcp.PromptListChangedRequest)
	resourceListChangedHandler  func(context.Context, *mcp.ResourceListChangedRequest)
	resourceUpdatedHandler      func(context.Context, *mcp.ResourceUpdatedNotificationRequest)
	loggingMessageHandler       func(context.Context, *mcp.LoggingMessageRequest)
	progressNotificationHandler func(context.Context, *mcp.ProgressNotificationClientRequest)
	createMessageHandler        func(context.Context, *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error)
	elicitationHandler          func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)

	mu      sync.RWMutex
	state   ProcessState
	err     error
	session *mcp.ClientSession
	cmd     *exec.Cmd // nil when using custom transport
	logger  *slog.Logger
}

// ProcessManagerConfig holds configuration for creating a ProcessManager.
type ProcessManagerConfig struct {
	// Command is the path to the executable.
	// Ignored if Transport is set.
	Command string
	// Args are the command-line arguments.
	// Ignored if Transport is set.
	Args []string
	// Env is optional environment variables (in "KEY=VALUE" format).
	// If nil, inherits from parent process.
	// Ignored if Transport is set.
	Env []string
	// WorkDir is the working directory for the subprocess.
	// If empty, inherits from parent process.
	// Ignored if Transport is set.
	WorkDir string
	// Logger for logging process events.
	Logger *slog.Logger

	// Transport is an optional pre-configured transport.
	// If set, no subprocess is started and Command/Args/Env/WorkDir are ignored.
	// This is primarily useful for testing with in-memory transports.
	Transport mcp.Transport

	// Notification handlers for events from the subprocess.
	// These are called when the subprocess sends notifications.
	ToolListChangedHandler      func(context.Context, *mcp.ToolListChangedRequest)
	PromptListChangedHandler    func(context.Context, *mcp.PromptListChangedRequest)
	ResourceListChangedHandler  func(context.Context, *mcp.ResourceListChangedRequest)
	ResourceUpdatedHandler      func(context.Context, *mcp.ResourceUpdatedNotificationRequest)
	LoggingMessageHandler       func(context.Context, *mcp.LoggingMessageRequest)
	ProgressNotificationHandler func(context.Context, *mcp.ProgressNotificationClientRequest)

	// Sampling handler - called when subprocess requests LLM completion.
	// If nil, sampling requests will be rejected with an error.
	CreateMessageHandler func(context.Context, *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error)

	// Elicitation handler - called when subprocess requests user input.
	// If nil, elicitation requests will be rejected with an error.
	ElicitationHandler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
}

// NewProcessManager creates a new ProcessManager with the given configuration.
func NewProcessManager(cfg ProcessManagerConfig) *ProcessManager {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &ProcessManager{
		cmdPath:                     cfg.Command,
		cmdArgs:                     cfg.Args,
		env:                         cfg.Env,
		workDir:                     cfg.WorkDir,
		transport:                   cfg.Transport,
		toolListChangedHandler:      cfg.ToolListChangedHandler,
		promptListChangedHandler:    cfg.PromptListChangedHandler,
		resourceListChangedHandler:  cfg.ResourceListChangedHandler,
		resourceUpdatedHandler:      cfg.ResourceUpdatedHandler,
		loggingMessageHandler:       cfg.LoggingMessageHandler,
		progressNotificationHandler: cfg.ProgressNotificationHandler,
		createMessageHandler:        cfg.CreateMessageHandler,
		elicitationHandler:          cfg.ElicitationHandler,
		state:                       StateStopped,
		logger:                      logger,
	}
}

// Start spawns the subprocess and establishes an MCP client connection.
// It returns an error if the subprocess fails to start or the MCP handshake fails.
// If a custom Transport was provided in the config, no subprocess is started.
func (pm *ProcessManager) Start(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.state == StateRunning {
		return fmt.Errorf("process already running")
	}

	// Configure client with notification handlers
	clientOpts := &mcp.ClientOptions{
		ToolListChangedHandler:      pm.toolListChangedHandler,
		PromptListChangedHandler:    pm.promptListChangedHandler,
		ResourceListChangedHandler:  pm.resourceListChangedHandler,
		ResourceUpdatedHandler:      pm.resourceUpdatedHandler,
		LoggingMessageHandler:       pm.loggingMessageHandler,
		ProgressNotificationHandler: pm.progressNotificationHandler,
		CreateMessageHandler:        pm.createMessageHandler,
		ElicitationHandler:          pm.elicitationHandler,
	}

	client := mcp.NewClient(proxyImplementation, clientOpts)

	var transport mcp.Transport
	var cmd *exec.Cmd

	if pm.transport != nil {
		// Use pre-configured transport (for testing)
		pm.logger.Info("using pre-configured transport")
		transport = pm.transport
	} else {
		// Start subprocess with CommandTransport
		pm.logger.Info("starting subprocess",
			"command", pm.cmdPath,
			"args", pm.cmdArgs,
		)

		cmd = exec.CommandContext(ctx, pm.cmdPath, pm.cmdArgs...) //nolint:gosec // Command path comes from trusted configuration

		// Set up environment
		if pm.env != nil {
			cmd.Env = pm.env
		} else {
			cmd.Env = os.Environ()
		}

		// Set working directory
		if pm.workDir != "" {
			cmd.Dir = pm.workDir
		}

		// Pipe stderr to our stderr for debugging
		cmd.Stderr = os.Stderr

		transport = &mcp.CommandTransport{
			Command: cmd,
		}
	}

	// Connect to the server (this starts the process if using CommandTransport)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		pm.state = StateCrashed
		pm.err = fmt.Errorf("failed to connect: %w", err)
		return pm.err
	}

	pm.cmd = cmd
	pm.session = session
	pm.state = StateRunning
	pm.err = nil

	// Log PID before starting monitor (while we still hold the lock)
	if cmd != nil && cmd.Process != nil {
		pm.logger.Info("subprocess started successfully", "pid", cmd.Process.Pid)
	} else {
		pm.logger.Info("connected successfully")
	}

	// Monitor session in background
	go pm.monitor()

	return nil
}

// monitor watches the subprocess and updates state if it crashes.
func (pm *ProcessManager) monitor() {
	// Wait for the session to terminate
	err := pm.session.Wait()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Only update state if we haven't already stopped
	if pm.state == StateRunning {
		pm.state = StateCrashed
		if err != nil {
			pm.err = fmt.Errorf("subprocess terminated: %w", err)
		} else {
			pm.err = fmt.Errorf("subprocess terminated unexpectedly")
		}
		pm.logger.Error("subprocess crashed",
			"error", pm.err,
		)
	}
}

// GetSession returns the MCP client session if the process is running.
// Returns an error if the process is not running or has crashed.
func (pm *ProcessManager) GetSession() (*mcp.ClientSession, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	switch pm.state {
	case StateStopped:
		return nil, fmt.Errorf("subprocess not started")
	case StateCrashed:
		return nil, pm.err
	case StateRunning:
		return pm.session, nil
	default:
		return nil, fmt.Errorf("unknown process state")
	}
}

// Stop gracefully stops the subprocess (if any) and closes the session.
func (pm *ProcessManager) Stop() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.state != StateRunning {
		return nil
	}

	pm.logger.Info("stopping")

	pm.state = StateStopped

	// Close the MCP session
	if pm.session != nil {
		err := pm.session.Close()
		pm.session = nil
		if err != nil {
			pm.logger.Warn("error closing session", "error", err)
		}
	}

	// If we have a subprocess, ensure it's terminated
	if pm.cmd != nil && pm.cmd.Process != nil {
		// Wait briefly for graceful exit, then kill if still running
		done := make(chan struct{})
		go func() {
			_ = pm.cmd.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Process exited cleanly
		case <-time.After(5 * time.Second):
			// Force kill
			pm.logger.Warn("subprocess did not exit gracefully, killing")
			_ = pm.cmd.Process.Kill()
			<-done
		}
	}

	return nil
}

// State returns the current state of the subprocess.
func (pm *ProcessManager) State() ProcessState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.state
}

// Error returns the last error that caused the process to crash.
func (pm *ProcessManager) Error() error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.err
}
