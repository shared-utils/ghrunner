package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

type StartCommand struct {
	RootDir string `name:"root-dir" type:"path" help:"Root directory" env:"ROOT_RUNNERS_DIR" default:"~/.github-runners"`
}

func (s *StartCommand) Run() error {
	runnerDirs, err := searchRunnerDirs(s.RootDir)
	if err != nil {
		return fmt.Errorf("failed to search runner dirs: %w", err)
	}

	fmt.Printf("Found %d runners\n", len(runnerDirs))
	if len(runnerDirs) == 0 {
		fmt.Println("No runners found, exiting")
		return nil
	}
	for _, dir := range runnerDirs {
		fmt.Printf("  - %s\n", dir)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Capture shutdown signal (SIGINT and SIGTERM)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	for _, dir := range runnerDirs {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			s.runRunnerLoop(ctx, dir)
		}(dir)
	}

	// Wait for shutdown signal
	<-sigCh
	fmt.Println("\nShutdown signal received, gracefully stopping all runners...")
	fmt.Println("(Press Ctrl+C again to force quit)")
	cancel()

	// Allow second Ctrl+C to force quit
	go func() {
		<-sigCh
		fmt.Println("\nForce quitting...")
		os.Exit(1)
	}()

	wg.Wait()
	fmt.Println("All runners stopped")
	return nil
}

func (s *StartCommand) runRunnerLoop(ctx context.Context, dir string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Clean up work directory before each run
		workDir := filepath.Join(dir, "_work")
		os.RemoveAll(workDir)

		// Use shell to load user's environment variables
		// macOS: /bin/zsh -lic
		// Linux: /bin/bash -lc
		var cmd *exec.Cmd
		runScript := fmt.Sprintf("cd %s && ./run.sh --once", dir)
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("/bin/zsh", "-lic", runScript)
		} else {
			cmd = exec.Command("/bin/bash", "-lc", runScript)
		}
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// Run child process in its own process group so Ctrl+C doesn't kill it directly
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		fmt.Printf("Starting runner: %s\n", dir)

		if err := cmd.Start(); err != nil {
			fmt.Printf("Runner %s failed to start: %v\n", dir, err)
			continue
		}

		// Wait for either process to finish or context to be cancelled
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-ctx.Done():
			// Context cancelled, gracefully stop the runner
			if cmd.Process != nil {
				fmt.Printf("Stopping runner: %s (waiting for current job to finish...)\n", dir)
				// Send SIGINT first for graceful shutdown
				syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)

				// Wait for process to exit with timeout
				select {
				case <-done:
					// Process exited gracefully
				case <-time.After(30 * time.Second):
					// Timeout, force kill
					fmt.Printf("Runner %s didn't stop in time, force killing...\n", dir)
					syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					<-done
				}
			}
			os.RemoveAll(workDir)
			fmt.Printf("Runner stopped: %s\n", dir)
			return
		case err := <-done:
			// Clean up work directory after each run
			os.RemoveAll(workDir)

			if err != nil {
				fmt.Printf("Runner %s error: %v\n", dir, err)
			}

			fmt.Printf("Runner %s completed, restarting...\n", dir)
		}
	}
}
