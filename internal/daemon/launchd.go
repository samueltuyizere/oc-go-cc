package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"text/template"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>serve</string>
        <string>--background</string>
        {{- if .ConfigPath}}
        <string>--config</string>
        <string>{{.ConfigPath}}</string>
        {{- end}}
        {{- if .Port}}
        <string>--port</string>
        <string>{{.Port}}</string>
        {{- end}}
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>{{.LogFile}}</string>

    <key>StandardErrorPath</key>
    <string>{{.LogFile}}</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>{{.EnvPath}}</string>
    </dict>
</dict>
</plist>
`

// PlistData holds the values interpolated into the launchd plist template.
type PlistData struct {
	Label      string
	BinaryPath string
	ConfigPath string
	Port       int
	LogFile    string
	EnvPath    string
}

// EnableAutostart creates the launchd plist and loads it.
func EnableAutostart(configPath string, port int) error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}
	if err := paths.EnsureConfigDir(); err != nil {
		return err
	}

	// Ensure LaunchAgents directory exists
	launchAgentsDir := filepath.Dir(paths.PlistPath)
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("cannot create LaunchAgents directory: %w", err)
	}

	// Build plist data
	envPath := os.Getenv("PATH")
	if envPath == "" {
		envPath = "/usr/local/bin:/usr/bin:/bin"
	}

	data := PlistData{
		Label:      LaunchAgent,
		BinaryPath: paths.BinaryPath,
		ConfigPath: configPath,
		Port:       port,
		LogFile:    paths.LogFile,
		EnvPath:    envPath,
	}

	// Render template
	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("cannot parse plist template: %w", err)
	}

	f, err := os.Create(paths.PlistPath)
	if err != nil {
		return fmt.Errorf("cannot create plist file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("cannot render plist: %w", err)
	}

	fmt.Printf("Autostart enabled. %s will start on login.\n", AppName)
	fmt.Printf("  Plist: %s\n", paths.PlistPath)

	// Load the plist with launchctl
	if err := loadPlist(paths.PlistPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load plist with launchctl: %v\n", err)
		fmt.Fprintf(os.Stderr, "The plist is installed and will load on next login.\n")
	} else {
		fmt.Println("Service loaded successfully.")
	}

	return nil
}

// DisableAutostart unloads and removes the launchd plist.
func DisableAutostart() error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}

	// Check if plist exists
	if _, err := os.Stat(paths.PlistPath); os.IsNotExist(err) {
		fmt.Println("Autostart is not enabled (no plist found)")
		return nil
	}

	// Unload the plist first
	if err := unloadPlist(paths.PlistPath); err != nil {
		fmt.Fprintf(os.Stderr, "note: could not unload plist: %v\n", err)
	}

	// Remove the plist file
	if err := os.Remove(paths.PlistPath); err != nil {
		return fmt.Errorf("cannot remove plist: %w", err)
	}

	fmt.Printf("Autostart disabled. Plist removed.\n")
	return nil
}

// AutostartStatus reports whether autostart is enabled.
func AutostartStatus() error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}

	plistExists := false
	if _, err := os.Stat(paths.PlistPath); err == nil {
		plistExists = true
	}

	if !plistExists {
		fmt.Println("Autostart: disabled (no plist found)")
		return nil
	}

	// Check if the service is currently loaded
	loaded := isPlistLoaded()

	if loaded {
		fmt.Println("Autostart: enabled (plist installed and loaded)")
	} else {
		fmt.Println("Autostart: enabled (plist installed, not currently loaded)")
	}
	fmt.Printf("  Plist: %s\n", paths.PlistPath)
	return nil
}

func loadPlist(plistPath string) error {
	// launchctl bootout first (in case it's already loaded), then bootstrap
	uid := strconv.Itoa(os.Getuid())
	target := "gui/" + uid + "/" + LaunchAgent

	// Ignore errors from bootout -- it might not be loaded
	_ = exec.Command("launchctl", "bootout", target).Run()
	return exec.Command("launchctl", "bootstrap", target, plistPath).Run()
}

func unloadPlist(plistPath string) error {
	uid := strconv.Itoa(os.Getuid())
	target := "gui/" + uid + "/" + LaunchAgent
	return exec.Command("launchctl", "bootout", target).Run()
}

func isPlistLoaded() bool {
	// launchctl list returns 0 if the service is loaded
	err := exec.Command("launchctl", "list", LaunchAgent).Run()
	return err == nil
}
