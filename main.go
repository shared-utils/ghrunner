package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
)

func main() {
	runnerDirs, err := searchRunnerDirs(".")
	if err != nil {
		panic(err)
	}

	fmt.Printf("Found %d runners\n", len(runnerDirs))
	if len(runnerDirs) == 0 {
		fmt.Println("No runners found, exiting")
		return
	}
	for _, dir := range runnerDirs {
		fmt.Printf("  - %s\n", dir)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Capture shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	var wg sync.WaitGroup
	for _, dir := range runnerDirs {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			runRunnerLoop(ctx, dir)
		}(dir)
	}

	// Wait for shutdown signal
	<-sigCh
	fmt.Println("\nShutdown signal received, stopping all runners...")
	cancel()

	wg.Wait()
	fmt.Println("All runners stopped")
}

func runRunnerLoop(ctx context.Context, dir string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Clean up work directory
		workDir := filepath.Join(dir, "_work")
		os.RemoveAll(workDir)

		cmd := exec.CommandContext(ctx, "./run.sh", "--once")
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Printf("Starting runner: %s\n", dir)
		err := cmd.Run()

		os.RemoveAll(workDir)

		if ctx.Err() != nil {
			return
		}

		if err != nil {
			fmt.Printf("Runner %s error: %v\n", dir, err)
		}

		fmt.Printf("Runner %s completed, restarting...\n", dir)
	}
}

func searchRunnerDirs(baseDir string) ([]string, error) {
	var result []string
	err := filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			runShPath := filepath.Join(path, "run.sh")
			if _, err := os.Stat(runShPath); err == nil {
				result = append(result, path)
				return filepath.SkipDir // Don't search inside runner directories
			}
		}
		return nil
	})
	return result, err
}
