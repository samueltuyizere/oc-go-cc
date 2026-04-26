package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	AppName     = "oc-go-cc"
	ConfigDir   = ".config/oc-go-cc"
	LaunchAgent = "com.opencode.oc-go-cc"
)

// Paths holds well-known directories and files for the app.
type Paths struct {
	ConfigDir  string // ~/.config/oc-go-cc
	PIDFile    string // ~/.config/oc-go-cc/oc-go-cc.pid
	LogFile    string // ~/.config/oc-go-cc/oc-go-cc.log
	PlistPath  string // ~/Library/LaunchAgents/com.opencode.oc-go-cc.plist
	BinaryPath string // absolute path to the running executable
}

// DefaultPaths computes paths from the user's home directory.
func DefaultPaths() (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot determine executable path: %w", err)
	}
	// Resolve symlinks so launchd gets the real binary
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve executable path: %w", err)
	}

	configDir := filepath.Join(home, ConfigDir)
	return &Paths{
		ConfigDir:  configDir,
		PIDFile:    filepath.Join(configDir, AppName+".pid"),
		LogFile:    filepath.Join(configDir, AppName+".log"),
		PlistPath:  filepath.Join(home, "Library", "LaunchAgents", LaunchAgent+".plist"),
		BinaryPath: execPath,
	}, nil
}

// EnsureConfigDir creates ~/.config/oc-go-cc/ if it does not exist.
func (p *Paths) EnsureConfigDir() error {
	return os.MkdirAll(p.ConfigDir, 0755)
}

// GetPID reads the PID from the PID file.
func GetPID(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}
	return pid, nil
}

// WritePID writes the given PID to a file.
func WritePID(pidPath string, pid int) error {
	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// IsProcessRunning checks if a process with the given PID is running.
func IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Send signal 0 to check if process exists
	err = process.Signal(os.Signal(nil))
	return err == nil
}

// StopProcess sends SIGTERM to a process and waits for it to exit.
func StopProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("cannot find process: %w", err)
	}

	if err := process.Signal(os.Signal(syscall.SIGTERM)); err != nil {
		return fmt.Errorf("cannot send SIGTERM: %w", err)
	}

	// Wait for the process to exit
	_, _ = process.Wait()
	return nil
}

// FindBinary returns the absolute path to the oc-go-cc binary.
func FindBinary() (string, error) {
	// First try to use the current executable
	execPath, err := os.Executable()
	if err == nil {
		execPath, err = filepath.EvalSymlinks(execPath)
		if err == nil {
			return execPath, nil
		}
	}

	// Fallback: search PATH for oc-go-cc
	cmd := exec.Command("which", AppName)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot find oc-go-cc binary: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
