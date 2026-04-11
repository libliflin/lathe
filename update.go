package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
)

// version is set at build time via -ldflags "-X main.version=<sha>"
var version = "dev"

const repoAPI = "https://api.github.com/repos/libliflin/lathe/releases/latest"

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func cmdVersion() {
	fmt.Printf("lathe %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
}

func cmdUpdate() {
	fmt.Printf("  Current version: %s\n", version)
	fmt.Println("  Checking for updates ...")

	resp, err := http.Get(repoAPI)
	if err != nil {
		die("check for updates: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		die("GitHub API returned %d", resp.StatusCode)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		die("parse release: %v", err)
	}

	// Tag is "v0.0.0-<sha>" — extract the sha
	latestVersion := release.TagName
	parts := strings.SplitN(latestVersion, "-", 2)
	latestSHA := latestVersion
	if len(parts) == 2 {
		latestSHA = parts[1]
	}

	if latestSHA == version {
		fmt.Println("  Already up to date.")
		return
	}

	fmt.Printf("  New version available: %s\n", latestSHA)

	// Find the right asset for this OS/arch
	assetName := fmt.Sprintf("lathe-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}

	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		die("no binary for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, latestVersion)
	}

	fmt.Printf("  Downloading %s ...\n", assetName)

	// Download to a temp file next to the current binary
	exe, err := os.Executable()
	if err != nil {
		die("resolve executable path: %v", err)
	}

	tmpFile := exe + ".update"
	if err := downloadFile(downloadURL, tmpFile); err != nil {
		os.Remove(tmpFile)
		die("download: %v", err)
	}

	// Make executable
	if err := os.Chmod(tmpFile, 0755); err != nil {
		os.Remove(tmpFile)
		die("chmod: %v", err)
	}

	// Swap: rename current binary out, rename new one in
	// On Unix, renaming over a running binary is fine
	if err := os.Rename(tmpFile, exe); err != nil {
		os.Remove(tmpFile)
		die("replace binary: %v", err)
	}

	fmt.Printf("  Updated to %s.\n", latestSHA)
}

func downloadFile(url, dst string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
