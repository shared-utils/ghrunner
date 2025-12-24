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

type StopCommand struct {
	RootDir string `name:"root-dir" type:"path" help:"Root directory" env:"ROOT_RUNNERS_DIR" default:"~/.github-runners"`
}

func (s *StopCommand) Run() error {
	switch runtime.GOOS {
	case "darwin":
		return s.stopMacOS()
	case "linux":
		return s.stopLinux()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (s *StopCommand) stopMacOS() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")

	label := "com.github.actions.runner"
	plistPath := filepath.Join(launchAgentsDir, label+".plist")

	// Check if plist exists
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("LaunchAgent not found. Run 'ghrunner enable' first.")
		return nil
	}

	// Unload the LaunchAgent
	cmd := exec.Command("launchctl", "unload", plistPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Might not be loaded, that's fine
		fmt.Println("LaunchAgent was not running")
		return nil
	}

	fmt.Println("Stopped ghrunner service")
	return nil
}

func (s *StopCommand) stopLinux() error {
	// Check if running as root
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	if currentUser.Uid != "0" {
		return fmt.Errorf("stop command on Linux requires root privileges. Please run with sudo")
	}

	runnerDirs, err := searchRunnerDirs(s.RootDir)
	if err != nil {
		return fmt.Errorf("failed to search runner dirs: %w", err)
	}

	// Find all unique orgs
	orgs := make(map[string]bool)
	for _, runnerDir := range runnerDirs {
		relPath, err := filepath.Rel(s.RootDir, runnerDir)
		if err != nil {
			continue
		}
		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) >= 1 {
			orgs[parts[0]] = true
		}
	}

	if len(orgs) == 0 {
		fmt.Println("No runners found")
		return nil
	}

	stopped := 0
	for org := range orgs {
		serviceName := fmt.Sprintf("ghrunner-%s", org)

		// Stop the service
		cmd := exec.Command("systemctl", "stop", serviceName)
		if err := cmd.Run(); err != nil {
			// Might not be running, that's fine
			continue
		}

		fmt.Printf("Stopped service: %s\n", serviceName)
		stopped++
	}

	fmt.Printf("\nStopped %d services.\n", stopped)
	return nil
}
