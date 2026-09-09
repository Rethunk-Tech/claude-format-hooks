package installer

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
