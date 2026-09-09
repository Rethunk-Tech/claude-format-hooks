package installer

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

const (
	releaseRepository = "Rethunk-Tech/claude-format-hooks"
	// releaseWorkflowPath is the only workflow whose attestations are
	// accepted. Without this binding any provenance attestation on the
	// repository counts, including one minted by some other workflow a
	// contributor adds later -- which is a far lower bar than "the release
	// workflow built these bytes".
	releaseWorkflowPath      = ".github/workflows/release.yml"
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

type releaseAttestations struct {
	Attestations []struct {
		Bundle struct {
			DSSEEnvelope struct {
				Payload string `json:"payload"`
			} `json:"dsseEnvelope"`
		} `json:"bundle"`
	} `json:"attestations"`
}

// provenanceStatement is the in-toto statement carried in an attestation's
// DSSE envelope: what was built, by which workflow, in which repository.
type provenanceStatement struct {
	Subject []struct {
		Digest map[string]string `json:"digest"`
	} `json:"subject"`
	Predicate struct {
		BuildDefinition struct {
			ExternalParameters struct {
				Workflow struct {
					Repository string `json:"repository"`
					Path       string `json:"path"`
				} `json:"workflow"`
			} `json:"externalParameters"`
		} `json:"buildDefinition"`
	} `json:"predicate"`
}

type upgradeConfig struct {
	client     *http.Client
	apiBaseURL string
	goos       string
	goarch     string
}

// Upgrade downloads the latest release binary for this runtime, verifies its
// published checksum and build provenance, and atomically replaces the
// installed hook binary.
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

	binary, err := fetchHTTP(cfg.client, binaryAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", assetName, err)
	}
	checksum, err := fetchHTTP(cfg.client, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", checksumName, err)
	}
	if err := verifyReleaseChecksum(checksum, assetName, binary); err != nil {
		return fmt.Errorf("verify %s: %w", assetName, err)
	}
	if err := verifyReleaseProvenance(cfg.client, cfg.apiBaseURL, binary); err != nil {
		return fmt.Errorf("verify build provenance for %s: %w; refusing to install an unattested binary. "+
			"Verify a download yourself with `gh attestation verify <file> --repo %s` and install it manually, "+
			"or wait for a release built with provenance", assetName, err, releaseRepository)
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

// verifyReleaseProvenance requires GitHub to hold a build-provenance
// attestation for these exact bytes under the release repository. The binary
// and its .sha256 ship in the same release, so the checksum proves transit
// only: whoever can rewrite the release rewrites both. Minting an attestation
// instead needs the release workflow's short-lived OIDC identity, which write
// access to release assets does not grant.
//
// ponytail: the trust anchor is TLS to the release API host, which this path
// already trusts for the release metadata and asset URLs it acts on
// (install.sh states that trust). Within that anchor this now binds the
// attestation to these exact bytes AND to the release workflow, decoding the
// in-toto statement out of the bundle's DSSE envelope with the standard
// library alone.
//
// Ceiling: the DSSE signature, its Fulcio certificate chain and the Rekor
// inclusion proof are not cryptographically verified, so this trusts the API
// to hand back an unforged bundle rather than proving it. Measured 2026-09:
// closing that means sigstore-go, which resolves to 71 modules (gRPC,
// protobuf, k8s.io/klog) against this module's current 7 -- a tenfold
// dependency increase, in a binary whose entire design premise is a 1-3ms
// cold start on every file write. Measured, not worth it. Revisit only if
// the threat model grows to include a compromised api.github.com, at which
// point the attestation list itself is forgeable too and the whole path
// needs rethinking, not just this check.
//
// An earlier note here recorded the endpoint returning "bundle": null and
// offloading to a snappy-compressed blob. Re-measured 2026-09-09: the bundle
// now comes back inline, which is what makes the workflow binding free.
func verifyReleaseProvenance(client *http.Client, baseURL string, binary []byte) error {
	digest := sha256.Sum256(binary)
	url := strings.TrimRight(baseURL, "/") + "/repos/" + releaseRepository +
		"/attestations/sha256:" + hex.EncodeToString(digest[:]) + "?predicate_type=provenance"
	body, err := fetchHTTP(client, url)
	if err != nil {
		return fmt.Errorf("fetch attestations: %w", err)
	}
	return verifyProvenanceBody(body, binary)
}

// verifyProvenanceBody is verifyReleaseProvenance minus the fetch, so the
// decision can be tested against a real captured API response instead of
// only against a hand-written one.
func verifyProvenanceBody(body, binary []byte) error {
	digest := sha256.Sum256(binary)
	// A digest with no attestation answers 404 on a repository that has never
	// attested anything and an empty list on one that has; both are a refusal.
	var attestations releaseAttestations
	if err := json.Unmarshal(body, &attestations); err != nil {
		return fmt.Errorf("decode attestations: %w", err)
	}
	if len(attestations.Attestations) == 0 {
		return fmt.Errorf("%s attests no build provenance for this download", releaseRepository)
	}

	// Any one attestation binding these bytes to the release workflow is
	// enough; a repository can hold several for the same digest.
	want := hex.EncodeToString(digest[:])
	var sawBundle bool
	for _, att := range attestations.Attestations {
		if att.Bundle.DSSEEnvelope.Payload == "" {
			continue
		}
		sawBundle = true
		stmt, err := decodeProvenance(att.Bundle.DSSEEnvelope.Payload)
		if err != nil {
			continue
		}
		if stmt.attests(want) {
			return nil
		}
	}
	if sawBundle {
		return fmt.Errorf("no attestation binds these bytes to %s in %s",
			releaseWorkflowPath, releaseRepository)
	}
	// The endpoint has returned bundle-less attestations before. Falling back
	// to existence alone is exactly the posture this path had before the
	// binding existed, and no weaker than the TLS anchor it already rests on
	// -- whereas refusing would break every upgrade on an API shape change.
	return nil
}

// decodeProvenance unwraps the base64 in-toto statement from a DSSE
// envelope payload.
func decodeProvenance(payload string) (provenanceStatement, error) {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return provenanceStatement{}, err
	}
	var stmt provenanceStatement
	if err := json.Unmarshal(raw, &stmt); err != nil {
		return provenanceStatement{}, err
	}
	return stmt, nil
}

// attests reports whether this statement says the release workflow, in the
// release repository, built exactly these bytes. All three must hold: a
// digest match alone would accept an attestation minted by any workflow,
// and a workflow match alone would accept one for different bytes.
func (s provenanceStatement) attests(wantDigest string) bool {
	workflow := s.Predicate.BuildDefinition.ExternalParameters.Workflow
	if workflow.Path != releaseWorkflowPath {
		return false
	}
	// The statement spells the repository as a URL; compare the owner/name
	// tail rather than pinning github.com's hostname.
	if !strings.HasSuffix(strings.TrimSuffix(workflow.Repository, "/"), "/"+releaseRepository) {
		return false
	}
	return slices.ContainsFunc(s.Subject, func(sub struct {
		Digest map[string]string `json:"digest"`
	}) bool {
		return strings.EqualFold(sub.Digest["sha256"], wantDigest)
	})
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
	// Reject on the advertised size before transferring: the LimitReader
	// below bounds the damage, but only after pulling down the full cap.
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
