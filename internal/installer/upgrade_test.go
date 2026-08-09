package installer

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUpgradeHappyPathPreservesModeAndSettings(t *testing.T) {
	binary := []byte("new release binary\n")
	server, _, binaryRequests, checksumRequests := upgradeTestServer(t, binary, binary)
	defer server.Close()

	dir := t.TempDir()
	target := HookBinaryPath(dir)
	oldBinary := []byte("old release binary\n")
	if err := os.WriteFile(target, oldBinary, 0o751); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, "settings.json")
	settings := []byte(`{"hooks":{"PostToolUse":[]}}`)
	if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := upgradeWithConfig(Options{
		BinPath:      target,
		SettingsPath: settingsPath,
	}, false, &out, upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, binary) {
		t.Fatalf("upgraded binary = %q, want %q", got, binary)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o751); got != want {
		t.Fatalf("upgraded mode = %o, want %o", got, want)
	}
	gotSettings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotSettings, settings) {
		t.Fatalf("settings changed from %q to %q", settings, gotSettings)
	}
	if got := binaryRequests.Load(); got != 1 {
		t.Fatalf("binary requests = %d, want 1", got)
	}
	if got := checksumRequests.Load(); got != 1 {
		t.Fatalf("checksum requests = %d, want 1", got)
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary replacement file still exists: %v", err)
	}
	if !strings.Contains(out.String(), "upgraded") {
		t.Fatalf("upgrade output = %q, want success message", out.String())
	}
}

func TestParseReleaseChecksumRequiresNamedAsset(t *testing.T) {
	assetName := "format-dispatch-linux-amd64"
	digest := sha256.Sum256([]byte("release binary"))

	tests := []struct {
		name         string
		checksumFile string
		wantErr      bool
	}{
		{
			name:         "digest only",
			checksumFile: fmt.Sprintf("%x\n", digest),
			wantErr:      true,
		},
		{
			name:         "wrong asset name",
			checksumFile: fmt.Sprintf("%x  other-binary\n", digest),
			wantErr:      true,
		},
		{
			name:         "named asset",
			checksumFile: fmt.Sprintf("%x  %s\n", digest, assetName),
		},
		{
			name:         "starred asset name",
			checksumFile: fmt.Sprintf("%x  *%s\n", digest, assetName),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseReleaseChecksum([]byte(tt.checksumFile), assetName)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseReleaseChecksum error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, digest[:]) {
				t.Fatalf("parsed digest = %x, want %x", got, digest)
			}
		})
	}
}

func TestUpgradeChecksumMismatchLeavesBinaryUntouched(t *testing.T) {
	binary := []byte("new release binary\n")
	server, _, _, _ := upgradeTestServer(t, binary, []byte("different binary\n"))
	defer server.Close()

	dir := t.TempDir()
	target := HookBinaryPath(dir)
	oldBinary := []byte("old release binary\n")
	if err := os.WriteFile(target, oldBinary, 0o700); err != nil {
		t.Fatal(err)
	}

	err := upgradeWithConfig(Options{BinPath: target}, false, nil, upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("upgrade error = %v, want checksum mismatch", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldBinary) {
		t.Fatalf("binary changed after checksum mismatch: %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Fatalf("binary mode = %o, want %o", got, want)
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary replacement file still exists: %v", err)
	}
}

func TestUpgradeDryRunDoesNotWriteOrDownloadAssets(t *testing.T) {
	binary := []byte("new release binary\n")
	server, releaseRequests, binaryRequests, checksumRequests := upgradeTestServer(t, binary, binary)
	defer server.Close()

	dir := t.TempDir()
	target := HookBinaryPath(dir)
	oldBinary := []byte("old release binary\n")
	if err := os.WriteFile(target, oldBinary, 0o751); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := upgradeWithConfig(Options{BinPath: target}, true, &out, upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldBinary) {
		t.Fatalf("dry-run changed binary: %q", got)
	}
	if got := releaseRequests.Load(); got != 0 {
		t.Fatalf("dry-run release requests = %d, want 0", got)
	}
	if got := binaryRequests.Load(); got != 0 {
		t.Fatalf("dry-run binary requests = %d, want 0", got)
	}
	if got := checksumRequests.Load(); got != 0 {
		t.Fatalf("dry-run checksum requests = %d, want 0", got)
	}
	if !strings.Contains(out.String(), "--dry-run") || !strings.Contains(out.String(), target) {
		t.Fatalf("dry-run output = %q, want plan with target", out.String())
	}
}

func TestUpgradeUsesReleaseAPIEnvironmentOverride(t *testing.T) {
	binary := []byte("environment release binary\n")
	server, _, _, _ := upgradeTestServer(t, binary, binary)
	defer server.Close()
	t.Setenv("CLAUDE_FORMAT_HOOKS_RELEASE_API", server.URL)

	target := HookBinaryPath(t.TempDir())
	if err := Upgrade(Options{BinPath: target}, false, nil); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, binary) {
		t.Fatalf("upgraded binary = %q, want %q", got, binary)
	}
}

func TestFetchHTTPRejectsOversizedContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", maxUpgradeDownloadBytes+1))
		_, _ = w.Write([]byte("body is not read"))
	}))
	defer server.Close()

	_, err := fetchHTTP(server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum download size") {
		t.Fatalf("fetchHTTP error = %v, want download size error", err)
	}
}

func TestUpgradeOversizedBinaryLeavesInstalledBinaryUntouched(t *testing.T) {
	oversized := bytes.Repeat([]byte("x"), maxUpgradeDownloadBytes+1)
	server, _, _, _ := upgradeTestServer(t, oversized, nil)
	defer server.Close()

	target := HookBinaryPath(t.TempDir())
	oldBinary := []byte("installed binary\n")
	if err := os.WriteFile(target, oldBinary, 0o751); err != nil {
		t.Fatal(err)
	}

	err := upgradeWithConfig(Options{BinPath: target}, false, nil, upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum download size") {
		t.Fatalf("upgrade error = %v, want download size error", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldBinary) {
		t.Fatalf("binary changed after oversized download: %q", got)
	}
}

func TestUpgradeFreshInstallUsesExecutableMode(t *testing.T) {
	binary := []byte("fresh release binary\n")
	server, _, _, _ := upgradeTestServer(t, binary, binary)
	defer server.Close()

	target := HookBinaryPath(t.TempDir())
	err := upgradeWithConfig(Options{BinPath: target}, false, nil, upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
		t.Fatalf("fresh binary mode = %o, want %o", got, want)
	}
}

func TestUpgradeMissingBinaryAsset(t *testing.T) {
	server, _, _, _ := upgradeTestServerForTarget(t, []byte("binary\n"), []byte("binary\n"),
		runtime.GOOS, runtime.GOARCH, false, true)
	defer server.Close()

	err := upgradeWithConfig(Options{BinPath: HookBinaryPath(t.TempDir())}, false, nil, upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	})
	if err == nil || !strings.Contains(err.Error(), "has no asset") {
		t.Fatalf("upgrade error = %v, want missing binary asset error", err)
	}
}

func TestUpgradeMissingChecksumAsset(t *testing.T) {
	server, _, _, _ := upgradeTestServerForTarget(t, []byte("binary\n"), []byte("binary\n"),
		runtime.GOOS, runtime.GOARCH, true, false)
	defer server.Close()

	err := upgradeWithConfig(Options{BinPath: HookBinaryPath(t.TempDir())}, false, nil, upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	})
	if err == nil || !strings.Contains(err.Error(), "has no asset") {
		t.Fatalf("upgrade error = %v, want missing checksum asset error", err)
	}
}

func TestUpgradeWindowsAssetPairOnLinux(t *testing.T) {
	binary := []byte("windows release binary\n")
	server, _, _, _ := upgradeTestServerForTarget(t, binary, binary, "windows", "amd64", true, true)
	defer server.Close()

	target := HookBinaryPath(t.TempDir())
	err := upgradeWithConfig(Options{BinPath: target}, false, nil, upgradeConfig{
		client:     server.Client(),
		apiBaseURL: server.URL,
		goos:       "windows",
		goarch:     "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, binary) {
		t.Fatalf("windows asset result = %q, want %q", got, binary)
	}
}

func upgradeTestServer(t *testing.T, binary, checksumBinary []byte) (*httptest.Server, *atomic.Int32, *atomic.Int32, *atomic.Int32) {
	return upgradeTestServerForTarget(t, binary, checksumBinary, runtime.GOOS, runtime.GOARCH, true, true)
}

func upgradeTestServerForTarget(t *testing.T, binary, checksumBinary []byte, goos, goarch string, includeBinary, includeChecksum bool) (*httptest.Server, *atomic.Int32, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	assetName := fmt.Sprintf("format-dispatch-%s-%s", goos, goarch)
	if goos == "windows" {
		assetName += ".exe"
	}
	checksum := sha256.Sum256(checksumBinary)
	checksumFile := []byte(fmt.Sprintf("%x  %s\n", checksum, assetName))
	var releaseRequests atomic.Int32
	var binaryRequests atomic.Int32
	var checksumRequests atomic.Int32
	assetJSON := make([]string, 0, 2)
	if includeBinary {
		assetJSON = append(assetJSON, fmt.Sprintf(`{"name":%q,"browser_download_url":%q}`, assetName, "__SERVER__/binary"))
	}
	if includeChecksum {
		assetJSON = append(assetJSON, fmt.Sprintf(`{"name":%q,"browser_download_url":%q}`, assetName+".sha256", "__SERVER__/checksum"))
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + releaseRepository + "/releases/latest":
			releaseRequests.Add(1)
			assets := strings.ReplaceAll(strings.Join(assetJSON, ","), "__SERVER__", server.URL)
			_, _ = fmt.Fprintf(w, `{"tag_name":"v9.9.9","assets":[%s]}`, assets)
		case "/binary":
			binaryRequests.Add(1)
			_, _ = w.Write(binary)
		case "/checksum":
			checksumRequests.Add(1)
			_, _ = w.Write(checksumFile)
		default:
			http.NotFound(w, r)
		}
	}))
	return server, &releaseRequests, &binaryRequests, &checksumRequests
}
