package installer

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// ghVerifyTimeout bounds the whole gh round trip. It reaches Sigstore's
// trust root and Rekor, not just api.github.com, so it is given more room
// than the release fetches -- but an upgrade must not hang on it.
const ghVerifyTimeout = 60 * time.Second

// signerWorkflow is the --signer-workflow value that pins an attestation to
// the workflow that publishes releases, in gh's owner/repo/path spelling.
var signerWorkflow = releaseRepository + "/" + releaseWorkflowPath

// verifyWithGH runs `gh attestation verify` over the downloaded bytes when
// the GitHub CLI is both installed and authenticated.
//
// This is the full cryptographic check the inline attestation binding
// cannot do: DSSE signature, the Fulcio certificate chain, and the Rekor
// transparency log inclusion proof, with the signing identity pinned to the
// release workflow. It costs no Go dependencies at all -- the same trade
// this project already makes for every formatter it shells out to, and the
// command HUMANS.md already tells operators to run by hand.
//
// available is false when gh is absent or unauthenticated, which are
// ordinary states rather than verification failures: `gh attestation
// verify` needs a token even for a public repository (it exits 4 without
// one). Only a verdict from a gh that could actually run is trusted, so an
// operator without gh gets exactly the inline binding and no false refusal.
func verifyWithGH(binaryPath string, out io.Writer) (available bool, err error) {
	gh, lookErr := exec.LookPath("gh")
	if lookErr != nil {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), ghVerifyTimeout)
	defer cancel()

	// Asking first keeps the verdict unambiguous. Without this an
	// unauthenticated gh and a tampered binary are both "non-zero exit",
	// and guessing between them either refuses good upgrades or accepts
	// bad ones.
	if err := exec.CommandContext(ctx, gh, "auth", "status").Run(); err != nil { //nolint:gosec // gh is a LookPath result; every argument is a package constant
		return false, nil
	}

	cmd := exec.CommandContext(ctx, gh, "attestation", "verify", binaryPath, //nolint:gosec // gh is a LookPath result; binaryPath is our own CreateTemp name and the rest are package constants
		"--repo", releaseRepository, "--signer-workflow", signerWorkflow)
	if output, err := cmd.CombinedOutput(); err != nil {
		return true, fmt.Errorf("gh attestation verify: %w: %s", err, firstLine(output))
	}
	_, _ = fmt.Fprintf(out, "==> verified build provenance with gh (signer %s)\n", signerWorkflow)
	return true, nil
}

// writeTempBinary stages the downloaded bytes so gh has a path to verify.
// gh reads a file, and the download only ever existed in memory.
func writeTempBinary(binary []byte) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "format-dispatch-verify-*")
	if err != nil {
		return "", nil, err
	}
	name := f.Name()
	cleanup = func() { _ = os.Remove(name) }
	if _, err := f.Write(binary); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return name, cleanup, nil
}
