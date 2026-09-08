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

	"github.com/go-quicktest/qt"
)

func TestUpgradeHappyPathPreservesModeAndSettings(t *testing.T) {
	binary := []byte("new release binary\n")
	server, _, binaryRequests, checksumRequests := upgradeTestServer(t, binary, binary)
	defer server.Close()

	oldBinary := []byte("old release binary\n")
	target := installedBinary(t, oldBinary, 0o751)
	settingsPath := filepath.Join(filepath.Dir(target), "settings.json")
	settings := []byte(`{"hooks":{"PostToolUse":[]}}`)
	qt.Assert(t, qt.IsNil(os.WriteFile(settingsPath, settings, 0o600)))

	var out bytes.Buffer
	err := upgradeWithConfig(Options{
		BinPath:      target,
		SettingsPath: settingsPath,
	}, false, &out, releaseConfig(server))
	if err != nil {
		t.Fatal(err)
	}

	assertFileIs(t, target, binary, "upgraded binary")
	assertPerm(t, target, 0o751)
	assertFileIs(t, settingsPath, settings, "upgrade must not touch settings")
	assertRequests(t, "binary", binaryRequests, 1)
	assertRequests(t, "checksum", checksumRequests, 1)
	assertNoScratchLeftBehind(t, target)
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

	oldBinary := []byte("old release binary\n")
	target := installedBinary(t, oldBinary, 0o700)

	err := upgradeWithConfig(Options{BinPath: target}, false, nil, releaseConfig(server))
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("upgrade error = %v, want checksum mismatch", err)
	}

	assertFileIs(t, target, oldBinary, "binary changed after checksum mismatch")
	assertPerm(t, target, 0o700)
	assertNoScratchLeftBehind(t, target)
}

func TestUpgradeDryRunDoesNotWriteOrDownloadAssets(t *testing.T) {
	binary := []byte("new release binary\n")
	server, releaseRequests, binaryRequests, checksumRequests := upgradeTestServer(t, binary, binary)
	defer server.Close()

	oldBinary := []byte("old release binary\n")
	target := installedBinary(t, oldBinary, 0o751)

	var out bytes.Buffer
	err := upgradeWithConfig(Options{BinPath: target}, true, &out, releaseConfig(server))
	if err != nil {
		t.Fatal(err)
	}
	assertFileIs(t, target, oldBinary, "dry-run changed binary")
	assertRequests(t, "release", releaseRequests, 0)
	assertRequests(t, "binary", binaryRequests, 0)
	assertRequests(t, "checksum", checksumRequests, 0)
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

	assertFileIs(t, target, binary, "upgraded binary")
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

func TestFetchHTTPReportsOversizedNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", maxUpgradeDownloadBytes+1))
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	_, err := fetchHTTP(server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("fetchHTTP error = %v, want HTTP error", err)
	}
	if strings.Contains(err.Error(), "exceeds maximum download size") {
		t.Fatalf("fetchHTTP error = %v, want status error before size error", err)
	}
}

func TestFetchHTTPCapsOversizedNon2xxBody(t *testing.T) {
	const sentinel = "oversized-error-body-sentinel"
	status := http.StatusBadGateway
	body := append(bytes.Repeat([]byte("e"), maxUpgradeErrorBodyBytes), []byte(sentinel)...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	_, err := fetchHTTP(server.Client(), server.URL)
	if err == nil {
		t.Fatal("fetchHTTP error = nil, want HTTP error")
	}
	errText := err.Error()
	if !strings.Contains(errText, fmt.Sprintf("HTTP %d", status)) {
		t.Fatalf("fetchHTTP error = %v, want HTTP %d", err, status)
	}
	if strings.Contains(errText, "exceeds maximum download size") {
		t.Fatalf("fetchHTTP error = %v, want status error before size error", err)
	}
	if strings.Contains(errText, sentinel) {
		t.Fatalf("fetchHTTP error = %v, want body capped before sentinel", err)
	}
	prefix := fmt.Sprintf("HTTP %d %s: ", status, http.StatusText(status))
	detail := strings.TrimPrefix(errText, prefix)
	if detail == errText {
		t.Fatalf("fetchHTTP error = %v, want status-shaped error", err)
	}
	if len(detail) > maxUpgradeErrorBodyBytes {
		t.Fatalf("error detail length = %d, want <= %d", len(detail), maxUpgradeErrorBodyBytes)
	}
}

func TestFetchHTTPReportsNon2xx(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "forbidden", status: http.StatusForbidden, body: "access denied"},
		{name: "not found", status: http.StatusNotFound, body: "missing release"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			_, err := fetchHTTP(server.Client(), server.URL)
			if err == nil {
				t.Fatal("fetchHTTP error = nil, want HTTP error")
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", tt.status)) {
				t.Fatalf("fetchHTTP error = %v, want HTTP %d", err, tt.status)
			}
			if !strings.Contains(err.Error(), tt.body) {
				t.Fatalf("fetchHTTP error = %v, want response body %q", err, tt.body)
			}
		})
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

	err := upgradeWithConfig(Options{BinPath: target}, false, nil, releaseConfig(server))
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum download size") {
		t.Fatalf("upgrade error = %v, want download size error", err)
	}
	assertFileIs(t, target, oldBinary, "binary changed after oversized download")
}

func TestUpgradeFreshInstallUsesExecutableMode(t *testing.T) {
	binary := []byte("fresh release binary\n")
	server, _, _, _ := upgradeTestServer(t, binary, binary)
	defer server.Close()

	target := HookBinaryPath(filepath.Join(t.TempDir(), "nested", "bin"))
	err := upgradeWithConfig(Options{BinPath: target}, false, nil, releaseConfig(server))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
			t.Fatalf("fresh binary mode = %o, want %o", got, want)
		}
		parentInfo, err := os.Stat(filepath.Dir(target))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := parentInfo.Mode().Perm(), os.FileMode(0o750); got != want {
			t.Fatalf("fresh binary parent mode = %o, want %o", got, want)
		}
	}
}

func TestUpgradeMissingBinaryAsset(t *testing.T) {
	server, _, _, _ := upgradeTestServerForTarget(t, []byte("binary\n"), []byte("binary\n"),
		runtime.GOOS, runtime.GOARCH, false, true)
	defer server.Close()

	err := upgradeWithConfig(Options{BinPath: HookBinaryPath(t.TempDir())}, false, nil, releaseConfig(server))
	if err == nil || !strings.Contains(err.Error(), "has no asset") {
		t.Fatalf("upgrade error = %v, want missing binary asset error", err)
	}
}

func TestUpgradeMissingChecksumAsset(t *testing.T) {
	server, _, _, _ := upgradeTestServerForTarget(t, []byte("binary\n"), []byte("binary\n"),
		runtime.GOOS, runtime.GOARCH, true, false)
	defer server.Close()

	err := upgradeWithConfig(Options{BinPath: HookBinaryPath(t.TempDir())}, false, nil, releaseConfig(server))
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
	assertFileIs(t, target, binary, "windows asset result")
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
