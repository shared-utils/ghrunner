package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

type DisableCommand struct {
	RootDir string `name:"root-dir" type:"path" help:"Root directory" env:"ROOT_RUNNERS_DIR" default:"~/.github-runners"`
}

func (d *DisableCommand) Run() error {
	switch runtime.GOOS {
	case "darwin":
		return d.disableMacOS()
	case "linux":
		return d.disableLinux()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (d *DisableCommand) disableMacOS() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")

	label := "com.github.actions.runner"
	plistPath := filepath.Join(launchAgentsDir, label+".plist")

	// Unload if loaded
	cmd := exec.Command("launchctl", "unload", plistPath)
	_ = cmd.Run() // Ignore errors if not loaded

	// Remove plist file
	if err := os.Remove(plistPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("LaunchAgent not found, nothing to disable")
			return nil
		}
		return fmt.Errorf("failed to remove %s: %w", plistPath, err)
	}

	fmt.Printf("Removed LaunchAgent: %s\n", plistPath)
	return nil
}

func (d *DisableCommand) disableLinux() error {
	// Check if running as root
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	if currentUser.Uid != "0" {
		return fmt.Errorf("disable command on Linux requires root privileges. Please run with sudo")
	}

	runnerDirs, err := searchRunnerDirs(d.RootDir)
	if err != nil {
		return fmt.Errorf("failed to search runner dirs: %w", err)
	}

	// Find all unique orgs
	orgs := make(map[string]bool)
	for _, runnerDir := range runnerDirs {
		relPath, err := filepath.Rel(d.RootDir, runnerDir)
		if err != nil {
			continue
		}
		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) >= 1 {
			orgs[parts[0]] = true
		}
	}

	if len(orgs) == 0 {
		fmt.Println("No runners found, nothing to disable")
		return nil
	}

	for org := range orgs {
		serviceName := fmt.Sprintf("ghrunner-%s", org)
		servicePath := filepath.Join("/etc/systemd/system", serviceName+".service")

		// Stop the service
		cmd := exec.Command("systemctl", "stop", serviceName)
		_ = cmd.Run() // Ignore errors if not running

		// Disable the service
		cmd = exec.Command("systemctl", "disable", serviceName)
		_ = cmd.Run() // Ignore errors if not enabled

		// Remove service file
		if err := os.Remove(servicePath); err != nil {
			if !os.IsNotExist(err) {
				fmt.Printf("Warning: failed to remove %s: %v\n", servicePath, err)
			}
		} else {
			fmt.Printf("Removed systemd service: %s\n", serviceName)
		}
	}

	// Reload systemd
	cmd := exec.Command("systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	fmt.Println("\nSystemd services removed.")
	return nil
}
