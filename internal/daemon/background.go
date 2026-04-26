package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
)

// BackgroundOpts are the options passed from the serve command.
type BackgroundOpts struct {
	ConfigPath string // --config flag value, may be empty
	Port       int    // --port flag value, 0 means default
}

// ForkIntoBackground starts the current binary as a detached background process.
// Uses nohup to detach from terminal and redirect output.
func ForkIntoBackground(opts BackgroundOpts) error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}
	if err := paths.EnsureConfigDir(); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	// Build args for nohup: nohup oc-go-cc serve --_daemonize [--config X] [--port N]
	args := []string{"serve", "--_daemonize"}
	if opts.ConfigPath != "" {
		args = append(args, "--config", opts.ConfigPath)
	}
	if opts.Port != 0 {
		args = append(args, "--port", strconv.Itoa(opts.Port))
	}

	// Open log file for the background process
	logFile, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	// Use nohup to detach from terminal - works cross-platform
	cmd := exec.Command("nohup", append([]string{paths.BinaryPath}, args...)...)
	cmd.Env = os.Environ()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Dir = "/" // Run from root to avoid any working directory issues

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background process: %w", err)
	}

	// Write PID file
	pid := cmd.Process.Pid
	if err := WritePID(paths.PIDFile, pid); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write PID file: %v\n", err)
	}

	fmt.Printf("Started %s in background (PID %d)\n", AppName, pid)
	fmt.Printf("  Log file: %s\n", paths.LogFile)
	fmt.Printf("  PID file: %s\n", paths.PIDFile)
	fmt.Printf("  Stop with: %s stop\n", AppName)

	return nil
}

// DaemonizeSetup is called by the child process (when --_daemonize is set).
// It redirects stdout/stderr to the log file and writes the PID file.
func DaemonizeSetup(paths *Paths) error {
	// Redirect stdout and stderr to log file
	logFile, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file: %w", err)
	}

	// Replace file descriptors so slog (which writes to os.Stdout) works
	os.Stdout = logFile
	os.Stderr = logFile

	// Re-initialize the default logger to use the new stdout
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Write PID file
	if err := WritePID(paths.PIDFile, os.Getpid()); err != nil {
		return fmt.Errorf("cannot write PID file: %w", err)
	}

	return nil
}
