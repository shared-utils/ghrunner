package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type SetupCommand struct {
	GithubToken      string   `name:"github-token" help:"GitHub token" env:"GITHUB_TOKEN" required:""`
	RootDir          string   `name:"root-dir" type:"path" help:"Root directory" default:"~/.github-runners"`
	Orgs             []string `name:"orgs" sep:"," help:"Organizations to deploy to" required:""`
	RunnersPerOrg    int      `name:"runners-per-org" help:"Number of runners per organization" default:"2"`
	DownloadDir      string   `name:"download-dir" type:"path" help:"Download directory" default:"~/Downloads"`
	AdditionalLabels []string `name:"additional-labels" sep:"," help:"Additional labels to add to the runners"`
}

// RunnerDownload represents a runner download option from GitHub API
type RunnerDownload struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	DownloadURL  string `json:"download_url"`
	Filename     string `json:"filename"`
}

// RegistrationToken represents the runner registration token from GitHub API
type RegistrationToken struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func (s *SetupCommand) Run() error {
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	// Step 1: Detect platform and architecture, download runner
	runnerPath, err := s.downloadRunner()
	if err != nil {
		return fmt.Errorf("failed to download runner: %w", err)
	}
	fmt.Printf("Runner downloaded to: %s\n", runnerPath)

	// Step 2: Setup runners for each org
	for _, org := range s.Orgs {
		fmt.Printf("\n=== Setting up runners for org: %s ===\n", org)

		// Get registration token for the org
		token, err := s.getRegistrationToken(org)
		if err != nil {
			return fmt.Errorf("failed to get registration token for org %s: %w", org, err)
		}

		// Create org directory
		orgDir := filepath.Join(s.RootDir, org)
		if err := os.MkdirAll(orgDir, 0755); err != nil {
			return fmt.Errorf("failed to create org directory %s: %w", orgDir, err)
		}

		// Setup each runner
		for i := 1; i <= s.RunnersPerOrg; i++ {
			runnerName := fmt.Sprintf("%s-%d", hostname, i)
			runnerDir := filepath.Join(orgDir, runnerName)

			fmt.Printf("  Setting up runner: %s\n", runnerName)

			// Clean up existing runner if exists
			if err := s.cleanupExistingRunner(runnerDir); err != nil {
				return fmt.Errorf("failed to cleanup existing runner %s: %w", runnerDir, err)
			}

			// Extract runner to directory
			if err := s.extractRunner(runnerPath, runnerDir); err != nil {
				return fmt.Errorf("failed to extract runner to %s: %w", runnerDir, err)
			}

			// Configure the runner
			if err := s.configureRunner(runnerDir, org, runnerName, token); err != nil {
				return fmt.Errorf("failed to configure runner %s: %w", runnerName, err)
			}

			fmt.Printf("  Runner %s configured successfully\n", runnerName)
		}
	}

	fmt.Println("\n=== Setup complete ===")
	return nil
}

func (s *SetupCommand) getRunnerDownloadURL() (string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go's GOOS/GOARCH to GitHub's naming
	var osName, archName string
	switch goos {
	case "darwin":
		osName = "osx"
	case "linux":
		osName = "linux"
	case "windows":
		osName = "win"
	default:
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}

	switch goarch {
	case "amd64":
		archName = "x64"
	case "arm64":
		archName = "arm64"
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	// Use the first org to get download URLs (they're the same for all orgs)
	if len(s.Orgs) == 0 {
		return "", fmt.Errorf("no organizations specified")
	}

	url := fmt.Sprintf("https://api.github.com/orgs/%s/actions/runners/downloads", s.Orgs[0])
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.GithubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get runner downloads: %s - %s", resp.Status, string(body))
	}

	var downloads []RunnerDownload
	if err := json.NewDecoder(resp.Body).Decode(&downloads); err != nil {
		return "", err
	}

	// Find matching download
	for _, d := range downloads {
		if d.OS == osName && d.Architecture == archName {
			return d.DownloadURL, nil
		}
	}

	return "", fmt.Errorf("no runner download found for %s/%s", osName, archName)
}

func (s *SetupCommand) downloadRunner() (string, error) {
	downloadURL, err := s.getRunnerDownloadURL()
	if err != nil {
		return "", err
	}

	fmt.Printf("Downloading runner from: %s\n", downloadURL)

	// Create download directory
	if err := os.MkdirAll(s.DownloadDir, 0755); err != nil {
		return "", err
	}

	// Extract filename from URL
	filename := filepath.Base(downloadURL)
	destPath := filepath.Join(s.DownloadDir, filename)

	// Check if already downloaded
	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("Runner already downloaded: %s\n", destPath)
		return destPath, nil
	}

	// Download the file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download runner: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return destPath, nil
}

func (s *SetupCommand) getRegistrationToken(org string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/orgs/%s/actions/runners/registration-token", org)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.GithubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get registration token: %s - %s", resp.Status, string(body))
	}

	var token RegistrationToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return "", err
	}

	return token.Token, nil
}

func (s *SetupCommand) cleanupExistingRunner(runnerDir string) error {
	if _, err := os.Stat(runnerDir); os.IsNotExist(err) {
		return nil
	}

	fmt.Printf("    Cleaning up existing runner at %s\n", runnerDir)

	// Simply remove the directory
	// The --replace flag in configureRunner will handle replacing the runner registration on GitHub
	return os.RemoveAll(runnerDir)
}

func (s *SetupCommand) extractRunner(tarPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *SetupCommand) configureRunner(runnerDir, org, runnerName, token string) error {
	configScript := filepath.Join(runnerDir, "config.sh")

	args := []string{
		"--url", fmt.Sprintf("https://github.com/%s", org),
		"--token", token,
		"--name", runnerName,
		"--unattended",
		"--replace",
	}

	if len(s.AdditionalLabels) > 0 {
		labels := ""
		for i, label := range s.AdditionalLabels {
			if i > 0 {
				labels += ","
			}
			labels += label
		}
		args = append(args, "--labels", labels)
	}

	cmd := exec.Command(configScript, args...)
	cmd.Dir = runnerDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
