package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
)

type Cli struct {
	Setup   SetupCommand   `cmd:"setup" help:"Setup the GitHub runners"`
	Enable  EnableCommand  `cmd:"enable" help:"Enable the GitHub runners (create LaunchAgent/systemd services)"`
	Disable DisableCommand `cmd:"disable" help:"Disable the GitHub runners (remove LaunchAgent/systemd services)"`
	Start   StartCommand   `cmd:"start" help:"Start the GitHub runners"`
	Stop    StopCommand    `cmd:"stop" help:"Stop the GitHub runners"`
}

func main() {
	var cli Cli
	ctx := kong.Parse(&cli,
		kong.Name("ghrunner"),
		kong.Description("GitHub runners manager"),
		kong.UsageOnError(),
	)
	if err := ctx.Run(); err != nil {
		log.Fatal(err)
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
