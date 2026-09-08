package installer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	releaseRepository        = "Rethunk-Tech/claude-format-hooks"
	releaseAPIBaseURL        = "https://api.github.com"
	maxUpgradeDownloadBytes  = 64 << 20
	maxUpgradeErrorBodyBytes = 4 << 10
)

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type upgradeConfig struct {
	client     *http.Client
	apiBaseURL string
	goos       string
	goarch     string
}

// Upgrade downloads the latest release binary for this runtime, verifies its
// published checksum, and atomically replaces the installed hook binary.
// Settings are intentionally not changed; the installed hook path remains
// wired even while its binary is upgraded.
func Upgrade(opts Options, dryRun bool, out io.Writer) error {
	apiBaseURL := os.Getenv("CLAUDE_FORMAT_HOOKS_RELEASE_API")
	if apiBaseURL == "" {
		apiBaseURL = releaseAPIBaseURL
	}
	return upgradeWithConfig(opts, dryRun, out, upgradeConfig{
		client:     &http.Client{Timeout: 30 * time.Second},
		apiBaseURL: apiBaseURL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	})
}

func upgradeWithConfig(opts Options, dryRun bool, out io.Writer, cfg upgradeConfig) error {
	if out == nil {
		out = io.Discard
	}
	if opts.BinPath == "" {
		return fmt.Errorf("binary path is empty")
	}
	target := HookBinaryPath(filepath.Dir(opts.BinPath))
	if cfg.client == nil {
		cfg.client = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.apiBaseURL == "" {
		cfg.apiBaseURL = releaseAPIBaseURL
	}
	if cfg.goos == "" || cfg.goarch == "" {
		return fmt.Errorf("runtime target is incomplete")
	}

	assetName := fmt.Sprintf("%s-%s-%s", hookBinaryName, cfg.goos, cfg.goarch)
	if cfg.goos == "windows" {
		assetName += ".exe"
	}
	checksumName := assetName + ".sha256"

	if dryRun {
		_, _ = fmt.Fprintf(out, "==> --dry-run: would download %s and replace %s (not written)\n",
			assetName, target)
		return nil
	}

	release, err := fetchLatestRelease(cfg.client, cfg.apiBaseURL)
	if err != nil {
		return err
	}
	binaryAsset, ok := findReleaseAsset(release.Assets, assetName)
	if !ok {
		return fmt.Errorf("latest release %q has no asset %q", release.TagName, assetName)
	}
	checksumAsset, ok := findReleaseAsset(release.Assets, checksumName)
	if !ok {
		return fmt.Errorf("latest release %q has no asset %q", release.TagName, checksumName)
	}
	if binaryAsset.BrowserDownloadURL == "" || checksumAsset.BrowserDownloadURL == "" {
		return fmt.Errorf("latest release %q has an asset without a download URL", release.TagName)
	}

	binary, err := fetchAsset(cfg.client, binaryAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", assetName, err)
	}
	checksum, err := fetchAsset(cfg.client, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", checksumName, err)
	}
	if err := verifyReleaseChecksum(checksum, assetName, binary); err != nil {
		return fmt.Errorf("verify %s: %w", assetName, err)
	}

	mode, err := existingBinaryMode(target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}
	if err := writeAtomic(target, binary, mode); err != nil {
		return fmt.Errorf("replace %s: %w", target, err)
	}
	_, _ = fmt.Fprintf(out, "==> upgraded %s to release %s\n", target, release.TagName)
	return nil
}

func fetchLatestRelease(client *http.Client, baseURL string) (githubRelease, error) {
	var release githubRelease
	url := strings.TrimRight(baseURL, "/") + "/repos/" + releaseRepository + "/releases/latest"
	body, err := fetchHTTP(client, url)
	if err != nil {
		return release, fmt.Errorf("fetch latest release: %w", err)
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return release, fmt.Errorf("decode latest release: %w", err)
	}
	return release, nil
}

func fetchAsset(client *http.Client, url string) ([]byte, error) {
	return fetchHTTP(client, url)
}

func fetchHTTP(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "format-dispatch")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxUpgradeErrorBodyBytes))
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = resp.Status
		}
		return nil, fmt.Errorf("HTTP %s: %s", resp.Status, detail)
	}
	if resp.ContentLength > maxUpgradeDownloadBytes {
		return nil, fmt.Errorf("response body exceeds maximum download size of %d bytes", maxUpgradeDownloadBytes)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxUpgradeDownloadBytes+1))
	if len(body) > maxUpgradeDownloadBytes {
		return nil, fmt.Errorf("response body exceeds maximum download size of %d bytes", maxUpgradeDownloadBytes)
	}
	if readErr != nil {
		return nil, readErr
	}
	return body, nil
}

func findReleaseAsset(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func verifyReleaseChecksum(checksumFile []byte, assetName string, binary []byte) error {
	expected, err := parseReleaseChecksum(checksumFile, assetName)
	if err != nil {
		return err
	}
	actual := sha256.Sum256(binary)
	if !bytes.Equal(expected, actual[:]) {
		return fmt.Errorf("checksum mismatch")
	}
	return nil
}

func parseReleaseChecksum(checksumFile []byte, assetName string) ([]byte, error) {
	for line := range strings.SplitSeq(string(checksumFile), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid checksum file")
		}
		if strings.TrimPrefix(fields[1], "*") != assetName {
			return nil, fmt.Errorf("checksum file names %q, not %q", strings.TrimPrefix(fields[1], "*"), assetName)
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size {
			return nil, fmt.Errorf("invalid sha256 digest")
		}
		return digest, nil
	}
	return nil, fmt.Errorf("checksum file is empty")
}

func existingBinaryMode(path string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return 0, fmt.Errorf("installed binary %s is not a regular file", path)
		}
		return info.Mode().Perm(), nil
	}
	if os.IsNotExist(err) {
		return 0o755, nil
	}
	return 0, fmt.Errorf("stat installed binary: %w", err)
}
