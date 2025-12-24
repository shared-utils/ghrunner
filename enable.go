package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

type EnableCommand struct {
	RootDir string `name:"root-dir" type:"path" help:"Root directory" env:"ROOT_RUNNERS_DIR" default:"~/.github-runners"`
}

// LaunchAgent plist template for macOS
// Single LaunchAgent that runs ghrunner start
const launchAgentTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.ExePath}}</string>
        <string>start</string>
        <string>--root-dir={{.RootDir}}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.LogPath}}/ghrunner.log</string>
    <key>StandardErrorPath</key>
    <string>{{.LogPath}}/ghrunner.error.log</string>
</dict>
</plist>
`

// systemd service template for Linux
// Each org gets its own service running as its own user
const systemdServiceTemplate = `[Unit]
Description=GitHub Actions Runner - {{.Org}}
After=network.target

[Service]
Type=simple
User={{.User}}
ExecStart={{.ExePath}} start --root-dir={{.OrgDir}}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

type LaunchAgentConfig struct {
	Label   string
	ExePath string
	RootDir string
	LogPath string
}

type SystemdServiceConfig struct {
	Org     string
	OrgDir  string
	User    string
	ExePath string
}

func (e *EnableCommand) Run() error {
	switch runtime.GOOS {
	case "darwin":
		return e.enableMacOS()
	case "linux":
		return e.enableLinux()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (e *EnableCommand) enableMacOS() error {
	runnerDirs, err := searchRunnerDirs(e.RootDir)
	if err != nil {
		return fmt.Errorf("failed to search runner dirs: %w", err)
	}

	if len(runnerDirs) == 0 {
		return fmt.Errorf("no runners found in %s", e.RootDir)
	}

	// Get executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Get LaunchAgents directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	// Create log directory
	logDir := filepath.Join(homeDir, "Library", "Logs", "ghrunner")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	tmpl, err := template.New("plist").Parse(launchAgentTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	label := "com.github.actions.runner"
	plistPath := filepath.Join(launchAgentsDir, label+".plist")

	config := LaunchAgentConfig{
		Label:   label,
		ExePath: exePath,
		RootDir: e.RootDir,
		LogPath: logDir,
	}

	file, err := os.Create(plistPath)
	if err != nil {
		return fmt.Errorf("failed to create plist file %s: %w", plistPath, err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, config); err != nil {
		return fmt.Errorf("failed to write plist file %s: %w", plistPath, err)
	}

	fmt.Printf("Created LaunchAgent: %s\n", plistPath)
	fmt.Printf("Executable: %s\n", exePath)
	fmt.Printf("Log files will be at: %s\n", logDir)
	fmt.Println("\nTo start: launchctl load " + plistPath)
	fmt.Println("To stop:  launchctl unload " + plistPath)
	return nil
}

func (e *EnableCommand) enableLinux() error {
	// Check if running as root
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	if currentUser.Uid != "0" {
		return fmt.Errorf("enable command on Linux requires root privileges. Please run with sudo")
	}

	runnerDirs, err := searchRunnerDirs(e.RootDir)
	if err != nil {
		return fmt.Errorf("failed to search runner dirs: %w", err)
	}

	if len(runnerDirs) == 0 {
		return fmt.Errorf("no runners found in %s", e.RootDir)
	}

	// Get executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	tmpl, err := template.New("systemd").Parse(systemdServiceTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Find all unique orgs
	orgs := make(map[string]bool)
	for _, runnerDir := range runnerDirs {
		relPath, err := filepath.Rel(e.RootDir, runnerDir)
		if err != nil {
			continue
		}
		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) >= 1 {
			orgs[parts[0]] = true
		}
	}

	for org := range orgs {
		// Create user for this org if not exists
		username := org
		if err := e.createLinuxUser(username); err != nil {
			return fmt.Errorf("failed to create user %s: %w", username, err)
		}

		// Change ownership of org's runner directory to the user
		orgDir := filepath.Join(e.RootDir, org)
		if err := e.chownRecursive(orgDir, username); err != nil {
			return fmt.Errorf("failed to change ownership of %s: %w", orgDir, err)
		}

		serviceName := fmt.Sprintf("ghrunner-%s", org)
		servicePath := filepath.Join("/etc/systemd/system", serviceName+".service")

		config := SystemdServiceConfig{
			Org:     org,
			OrgDir:  orgDir,
			User:    username,
			ExePath: exePath,
		}

		file, err := os.Create(servicePath)
		if err != nil {
			return fmt.Errorf("failed to create service file %s: %w", servicePath, err)
		}

		if err := tmpl.Execute(file, config); err != nil {
			file.Close()
			return fmt.Errorf("failed to write service file %s: %w", servicePath, err)
		}
		file.Close()

		// Enable the service
		cmd := exec.Command("systemctl", "enable", serviceName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to enable service %s: %w", serviceName, err)
		}

		fmt.Printf("Created and enabled systemd service: %s (user: %s)\n", serviceName, username)
	}

	// Reload systemd
	cmd := exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	fmt.Printf("\nExecutable: %s\n", exePath)
	fmt.Println("To start: sudo systemctl start ghrunner-<org>")
	fmt.Println("To stop:  sudo systemctl stop ghrunner-<org>")
	return nil
}

func (e *EnableCommand) createLinuxUser(username string) error {
	// Check if user already exists
	_, err := user.Lookup(username)
	if err == nil {
		fmt.Printf("User %s already exists\n", username)
		return nil
	}

	// Create user with bash shell (needed for environment loading) and home directory
	cmd := exec.Command("useradd", "--system", "--create-home", "--shell", "/bin/bash", username)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	fmt.Printf("Created system user: %s\n", username)
	return nil
}

func (e *EnableCommand) chownRecursive(path, username string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("failed to lookup user %s: %w", username, err)
	}

	cmd := exec.Command("chown", "-R", fmt.Sprintf("%s:%s", u.Uid, u.Gid), path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
