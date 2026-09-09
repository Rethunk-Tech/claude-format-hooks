package installer

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-quicktest/qt"
)

// attestationBody builds the response shape api.github.com actually
// returns, measured 2026-09-09 against the published v0.4.0 asset: an
// attestations list whose bundle carries the in-toto statement inline in a
// DSSE envelope. The real fixture for that release passes this same code
// path; it is not committed because pinning it would mean committing the
// 11MB binary whose digest it names.
func attestationBody(t *testing.T, digest, repository, workflowPath string) []byte {
	t.Helper()
	stmt := map[string]any{
		"_type":   "https://in-toto.io/Statement/v1",
		"subject": []any{map[string]any{"digest": map[string]string{"sha256": digest}}},
		"predicate": map[string]any{
			"buildDefinition": map[string]any{
				"externalParameters": map[string]any{
					"workflow": map[string]string{
						"repository": repository,
						"path":       workflowPath,
					},
				},
			},
		},
	}
	payload, err := json.Marshal(stmt)
	qt.Assert(t, qt.IsNil(err))

	body, err := json.Marshal(map[string]any{
		"attestations": []any{map[string]any{
			"bundle": map[string]any{
				"dsseEnvelope": map[string]string{
					"payload": base64.StdEncoding.EncodeToString(payload),
				},
			},
		}},
	})
	qt.Assert(t, qt.IsNil(err))
	return body
}

func TestVerifyProvenanceBody(t *testing.T) {
	binary := []byte("the released bytes")
	sum := sha256.Sum256(binary)
	digest := hex.EncodeToString(sum[:])
	repo := "https://github.com/" + releaseRepository

	cases := []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{
			name: "release workflow attesting these bytes",
			body: attestationBody(t, digest, repo, releaseWorkflowPath),
		},
		{
			// Without the workflow binding, any attestation on the
			// repository counts -- including one minted by a workflow a
			// contributor adds later.
			name:    "some other workflow in the same repository",
			body:    attestationBody(t, digest, repo, ".github/workflows/ci.yml"),
			wantErr: true,
		},
		{
			name:    "release workflow attesting different bytes",
			body:    attestationBody(t, hex.EncodeToString(make([]byte, 32)), repo, releaseWorkflowPath),
			wantErr: true,
		},
		{
			name:    "same workflow path in another repository",
			body:    attestationBody(t, digest, "https://github.com/attacker/claude-format-hooks", releaseWorkflowPath),
			wantErr: true,
		},
		{
			name:    "no attestations at all",
			body:    []byte(`{"attestations":[]}`),
			wantErr: true,
		},
		{
			name:    "malformed response",
			body:    []byte(`not json`),
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyProvenanceBody(tc.body, binary)
			if tc.wantErr {
				qt.Check(t, qt.IsNotNil(err))
				return
			}
			qt.Check(t, qt.IsNil(err))
		})
	}
}

// The endpoint has returned bundle-less attestations before. Falling back to
// existence alone is the posture this path had before the binding existed
// and is no weaker than the TLS anchor it already rests on, whereas
// refusing would break every upgrade on an API shape change.
func TestVerifyProvenanceAcceptsBundleLessAttestations(t *testing.T) {
	err := verifyProvenanceBody([]byte(`{"attestations":[{"bundle":{}}]}`), []byte("bytes"))
	qt.Check(t, qt.IsNil(err))
}

// A subject digest is compared case-insensitively: hex spelling is not part
// of the identity.
func TestVerifyProvenanceIgnoresDigestCase(t *testing.T) {
	binary := []byte("the released bytes")
	sum := sha256.Sum256(binary)
	upper := strings.ToUpper(hex.EncodeToString(sum[:]))
	body := attestationBody(t, upper, "https://github.com/"+releaseRepository, releaseWorkflowPath)
	qt.Check(t, qt.IsNil(verifyProvenanceBody(body, binary)))
}

// gh's verdict is only trusted when gh could actually run. An absent or
// unauthenticated gh is an ordinary state -- `gh attestation verify` needs
// a token even for a public repository -- and must degrade to the inline
// binding rather than refuse a good upgrade.
func TestVerifyWithGHTreatsAbsentGHAsUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	available, err := verifyWithGH(filepath.Join(t.TempDir(), "bin"), io.Discard)
	qt.Check(t, qt.IsFalse(available))
	qt.Check(t, qt.IsNil(err))
}

// fakeGH puts a gh on PATH whose `auth status` and `attestation verify`
// exit codes the test controls.
func fakeGH(t *testing.T, authExit, verifyExit int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-shell only")
	}
	dir := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
  auth) exit %d ;;
  attestation) echo "gh says so"; exit %d ;;
esac
exit 1
`, authExit, verifyExit)
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755)))
	t.Setenv("PATH", dir)
}

func TestVerifyWithGH(t *testing.T) {
	cases := []struct {
		name          string
		authExit      int
		verifyExit    int
		wantAvailable bool
		wantErr       bool
	}{
		{name: "authenticated and verified", wantAvailable: true},
		{name: "authenticated and refused", verifyExit: 1, wantAvailable: true, wantErr: true},
		// gh exits 4 when it has no token, even for a public repository.
		{name: "unauthenticated", authExit: 4, wantAvailable: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeGH(t, tc.authExit, tc.verifyExit)
			available, err := verifyWithGH(filepath.Join(t.TempDir(), "bin"), io.Discard)
			qt.Check(t, qt.Equals(available, tc.wantAvailable))
			qt.Check(t, qt.Equals(err != nil, tc.wantErr))
		})
	}
}

// The signer pin is what makes gh's check equivalent to the inline binding
// rather than merely "some attestation exists".
func TestSignerWorkflowPinsTheReleaseWorkflow(t *testing.T) {
	qt.Check(t, qt.Equals(signerWorkflow,
		"Rethunk-Tech/claude-format-hooks/.github/workflows/release.yml"))
}

func TestWriteTempBinaryRoundTrips(t *testing.T) {
	want := []byte("released bytes")
	path, cleanup, err := writeTempBinary(want)
	qt.Assert(t, qt.IsNil(err))
	got, err := os.ReadFile(path)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.DeepEquals(got, want))

	cleanup()
	_, statErr := os.Stat(path)
	qt.Check(t, qt.IsTrue(os.IsNotExist(statErr)), qt.Commentf("the staged copy must not linger"))
}

// gh always talks to github.com, so its verdict only applies when this tool
// is pointed there too. Pointed at a mirror or a test server via
// CLAUDE_FORMAT_HOOKS_RELEASE_API, gh would be judging a different origin's
// bytes, and a refusal would say nothing about the ones just downloaded.
func TestVerifyProvenanceSkipsGHForOverriddenAPI(t *testing.T) {
	binary := []byte("the released bytes")
	sum := sha256.Sum256(binary)
	body := attestationBody(t, hex.EncodeToString(sum[:]),
		"https://github.com/"+releaseRepository, releaseWorkflowPath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	// A gh that refuses everything. It must never be consulted here.
	fakeGH(t, 0, 1)

	var out strings.Builder
	qt.Check(t, qt.IsNil(verifyProvenance(server.Client(), server.URL, binary, &out)))
	qt.Check(t, qt.Not(qt.StringContains(out.String(), "gh")),
		qt.Commentf("gh must not be consulted for a non-github.com base URL"))
}
