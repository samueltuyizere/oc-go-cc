// Package main is the CLI entry point for the oc-go-cc proxy server.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"oc-go-cc/internal/config"
	"oc-go-cc/internal/daemon"
	"oc-go-cc/internal/server"
)

const (
	appName     = "oc-go-cc"
	pidFileName = "oc-go-cc.pid"
)

// Version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   appName,
		Short: "Proxy Claude Code requests to OpenCode Go API",
		Long: `oc-go-cc is a CLI proxy tool that allows you to use your OpenCode Go
subscription with Claude Code. It intercepts Claude Code's Anthropic API requests,
transforms them to OpenAI format, and forwards them to OpenCode Go.

Configuration is stored at ~/.config/oc-go-cc/config.json`,
		Version: version,
	}

	// Add subcommands.
	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(stopCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(validateCmd())
	rootCmd.AddCommand(modelsCmd())
	rootCmd.AddCommand(autostartCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// serveCmd returns the command to start the proxy server.
func serveCmd() *cobra.Command {
	var configPath string
	var port int
	var background bool
	var daemonize bool // hidden internal flag

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle background mode: fork and exit parent
			if background && !daemonize {
				opts := daemon.BackgroundOpts{
					ConfigPath: configPath,
					Port:       port,
				}
				return daemon.ForkIntoBackground(opts)
			}

			// Override config path if provided.
			if configPath != "" {
				_ = os.Setenv("OC_GO_CC_CONFIG", configPath)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Override port if provided via flag.
			if port != 0 {
				cfg.Port = port
			}

			// Daemonize setup (child process after re-exec)
			if daemonize {
				paths, err := daemon.DefaultPaths()
				if err != nil {
					return err
				}
				if err := paths.EnsureConfigDir(); err != nil {
					return err
				}
				if err := daemon.DaemonizeSetup(paths); err != nil {
					return err
				}
			}

			// Check if already running.
			pidPath := getPIDPath()
			if pid, err := daemon.GetPID(pidPath); err == nil {
				// Check if process is still running.
				if daemon.IsProcessRunning(pid) {
					return fmt.Errorf("server is already running (PID %d)", pid)
				}
				// Stale PID file, clean up.
				_ = os.Remove(pidPath)
			}

			// Write PID file.
			if err := daemon.WritePID(pidPath, os.Getpid()); err != nil {
				return fmt.Errorf("failed to write PID file: %w", err)
			}
			defer func() { _ = os.Remove(pidPath) }()

			// Create and start server.
			srv, err := server.NewServer(cfg)
			if err != nil {
				return fmt.Errorf("failed to create server: %w", err)
			}

			fmt.Printf("Starting %s v%s\n", appName, version)
			fmt.Printf("Listening on %s:%d\n", cfg.Host, cfg.Port)
			fmt.Printf("Forwarding to: %s\n", cfg.OpenCodeGo.BaseURL)
			fmt.Println()
			fmt.Println("Configure Claude Code with:")
			fmt.Printf("  export ANTHROPIC_BASE_URL=http://%s:%d\n", cfg.Host, cfg.Port)
			fmt.Println("  export ANTHROPIC_AUTH_TOKEN=unused")
			fmt.Println()

			return srv.Start()
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	cmd.Flags().IntVarP(&port, "port", "p", 0, "Override listen port")
	cmd.Flags().BoolVarP(&background, "background", "b", false, "Run as background daemon")
	cmd.Flags().BoolVar(&daemonize, "_daemonize", false, "Internal use only")
	_ = cmd.Flags().MarkHidden("_daemonize")

	return cmd
}

// stopCmd returns the command to stop the proxy server.
func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidPath := getPIDPath()
			pid, err := daemon.GetPID(pidPath)
			if err != nil {
				return fmt.Errorf("server is not running (no PID file)")
			}

			if err := daemon.StopProcess(pid); err != nil {
				return fmt.Errorf("failed to stop server: %w", err)
			}

			fmt.Printf("Sent stop signal to server (PID %d)\n", pid)
			_ = os.Remove(pidPath)
			return nil
		},
	}
}

// statusCmd returns the command to check server status.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidPath := getPIDPath()
			pid, err := daemon.GetPID(pidPath)
			if err != nil {
				fmt.Println("Server is not running")
				return nil
			}

			if !daemon.IsProcessRunning(pid) {
				fmt.Println("Server is not running (stale PID file)")
				_ = os.Remove(pidPath)
				return nil
			}

			fmt.Printf("Server is running (PID %d)\n", pid)
			return nil
		},
	}
}

// initCmd returns the command to create a default configuration file.
func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := getConfigDir()
			configPath := filepath.Join(configDir, "config.json")

			// Check if config already exists
			if _, err := os.Stat(configPath); err == nil {
				// Backup existing config with timestamp
				timestamp := time.Now().Format("20060102-150405")
				backupPath := configPath + ".backup." + timestamp

				if err := os.Rename(configPath, backupPath); err != nil {
					return fmt.Errorf("failed to backup existing config: %w", err)
				}

				fmt.Printf("Backed up existing config to %s\n", backupPath)
			}

			if err := os.MkdirAll(configDir, 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			if err := os.WriteFile(configPath, []byte(getDefaultConfig()), 0644); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			fmt.Printf("Created default config at %s\n", configPath)
			fmt.Println("Edit the file and add your OpenCode Go API key.")
			return nil
		},
	}
}

// validateCmd returns the command to validate the configuration file.
func validateCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath != "" {
				_ = os.Setenv("OC_GO_CC_CONFIG", configPath)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("invalid config: %w", err)
			}

			fmt.Println("Configuration is valid!")
			fmt.Printf("  Host: %s\n", cfg.Host)
			fmt.Printf("  Port: %d\n", cfg.Port)
			fmt.Printf("  API Key: %s...\n", maskString(cfg.APIKey, 8))
			fmt.Printf("  Base URL: %s\n", cfg.OpenCodeGo.BaseURL)
			fmt.Printf("  Models configured: %d\n", len(cfg.Models))
			fmt.Printf("  Fallback chains: %d\n", len(cfg.Fallbacks))
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	return cmd
}

// modelsCmd returns the command to list available models.
func modelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List available OpenCode Go models",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Available OpenCode Go models:")
			fmt.Println()
			fmt.Println("  Model ID           Endpoint Type")
			fmt.Println("  ─────────────────────────────────────────")
			fmt.Println("  glm-5.1            OpenAI-compatible")
			fmt.Println("  glm-5              OpenAI-compatible")
			fmt.Println("  kimi-k2.6          OpenAI-compatible")
			fmt.Println("  kimi-k2.5          OpenAI-compatible")
			fmt.Println("  mimo-v2.5-pro      OpenAI-compatible")
			fmt.Println("  mimo-v2.5          OpenAI-compatible")
			fmt.Println("  mimo-v2-pro        OpenAI-compatible")
			fmt.Println("  mimo-v2-omni       OpenAI-compatible")
			fmt.Println("  minimax-m2.7       Anthropic-compatible")
			fmt.Println("  minimax-m2.5       Anthropic-compatible")
			fmt.Println("  deepseek-v4-pro    OpenAI-compatible")
			fmt.Println("  deepseek-v4-flash  OpenAI-compatible")
			fmt.Println("  qwen3.6-plus       OpenAI-compatible")
			fmt.Println("  qwen3.5-plus       OpenAI-compatible")
			fmt.Println()
			fmt.Println("Use these model IDs in your config.json file.")
		},
	}
}

// getConfigDir returns the default configuration directory path.
func getConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "oc-go-cc")
}

// autostartCmd returns the command to manage autostart on login.
func autostartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autostart",
		Short: "Manage auto-start on login",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "enable",
		Short: "Enable auto-start on login",
		RunE: func(cmd *cobra.Command, args []string) error {
			var configPath string
			var port int
			if cmd.Flags().Changed("config") {
				configPath, _ = cmd.Flags().GetString("config")
			}
			if cmd.Flags().Changed("port") {
				port, _ = cmd.Flags().GetInt("port")
			}
			return daemon.EnableAutostart(configPath, port)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "disable",
		Short: "Disable auto-start on login",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.DisableAutostart()
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check auto-start status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.AutostartStatus()
		},
	})

	cmd.PersistentFlags().StringP("config", "c", "", "Path to config file")
	cmd.PersistentFlags().IntP("port", "p", 0, "Override listen port")

	return cmd
}

// getPIDPath returns the path to the PID file.
func getPIDPath() string {
	paths, err := daemon.DefaultPaths()
	if err != nil {
		// Fallback to temp dir if home dir cannot be determined
		return filepath.Join(os.TempDir(), pidFileName)
	}
	return paths.PIDFile
}

// maskString masks all but the first `visible` characters of a string.
func maskString(s string, visible int) string {
	if len(s) <= visible {
		return s
	}
	return s[:visible] + "..."
}

// getDefaultConfig returns a default configuration JSON template.
// Optimized for cost-efficiency: uses cheaper models by default, expensive ones only when needed.
func getDefaultConfig() string {
	return `{
  "api_key": "${OC_GO_CC_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "models": {
    "budget": {
      "provider": "opencode-go",
      "model_id": "qwen3.6-plus",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "background": {
      "provider": "opencode-go",
      "model_id": "qwen3.5-plus",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "default": {
      "provider": "opencode-go",
      "model_id": "kimi-k2.6",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m2.5",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "think": {
      "provider": "opencode-go",
      "model_id": "glm-5",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "glm-5.1",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "fast": {
      "provider": "opencode-go",
      "model_id": "qwen3.6-plus",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },
  "fallbacks": {
    "budget": [
      { "provider": "opencode-go", "model_id": "kimi-k2.6" },
      { "provider": "opencode-go", "model_id": "mimo-v2.5-pro" },
      { "provider": "opencode-go", "model_id": "mimo-v2-pro" }
    ],
    "background": [
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" },
      { "provider": "opencode-go", "model_id": "minimax-m2.5" }
    ],
    "default": [
      { "provider": "opencode-go", "model_id": "mimo-v2.5-pro" },
      { "provider": "opencode-go", "model_id": "mimo-v2-pro" },
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" }
    ],
    "long_context": [
      { "provider": "opencode-go", "model_id": "minimax-m2.7" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ],
    "think": [
      { "provider": "opencode-go", "model_id": "kimi-k2.6" },
      { "provider": "opencode-go", "model_id": "mimo-v2.5-pro" },
      { "provider": "opencode-go", "model_id": "mimo-v2-pro" }
    ],
    "complex": [
      { "provider": "opencode-go", "model_id": "glm-5" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ],
    "fast": [
      { "provider": "opencode-go", "model_id": "qwen3.5-plus" },
      { "provider": "opencode-go", "model_id": "minimax-m2.5" }
    ]
  },
  "opencode_go": {
    "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/go/v1/messages",
    "timeout_ms": 300000
  },
  "logging": {
    "level": "info",
    "requests": true
  }
}
`
}
