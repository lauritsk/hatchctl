package security

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	cosignoci "github.com/sigstore/cosign/v3/pkg/oci"
	"github.com/sigstore/sigstore-go/pkg/root"
)

func TestVerifyImageReturnsReasonWhenReferenceCannotBeParsed(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()

	result := VerifyImage(context.Background(), "not a ref%%")
	if result.Verified {
		t.Fatal("expected verification failure")
	}
	if !strings.Contains(result.Reason, "unsupported image reference") {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestVerifyImageReturnsReasonOnBadReference(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()

	parseReference = func(string, ...name.Option) (name.Reference, error) {
		return nil, errors.New("bad ref")
	}
	result := VerifyImage(context.Background(), "example.com/app:latest")
	if result.Verified || !strings.Contains(result.Reason, "bad ref") {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestVerifyImageUsesBundleVerifierWhenAvailable(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()

	parseReference = func(string, ...name.Option) (name.Reference, error) {
		return name.MustParseReference("ghcr.io/example/demo:latest"), nil
	}
	trustedRootFunc = func() (root.TrustedMaterial, error) {
		return nil, nil
	}
	bundleCalled := false
	verifyImageAttestations = func(_ context.Context, _ name.Reference, opts *cosign.CheckOpts, _ ...name.Option) ([]cosignoci.Signature, bool, error) {
		bundleCalled = true
		if opts == nil {
			t.Fatal("expected check options")
		}
		if !opts.NewBundleFormat {
			t.Fatal("expected bundle verification to be enabled")
		}
		if opts.ClaimVerifier == nil {
			t.Fatal("expected bundle claim verifier")
		}
		if len(opts.Identities) != 1 {
			t.Fatalf("unexpected identities %#v", opts.Identities)
		}
		if opts.Identities[0].Issuer != "https://token.actions.githubusercontent.com" {
			t.Fatalf("unexpected identity %#v", opts.Identities[0])
		}
		if !strings.Contains(opts.Identities[0].SubjectRegExp, "github.com/example/demo") {
			t.Fatalf("unexpected identity %#v", opts.Identities[0])
		}
		return nil, false, nil
	}
	verifyImageSignatures = func(_ context.Context, _ name.Reference, _ *cosign.CheckOpts) ([]cosignoci.Signature, bool, error) {
		t.Fatal("did not expect legacy signature verification when bundle verification succeeds")
		return nil, false, nil
	}

	result := VerifyImage(context.Background(), "ghcr.io/example/demo:latest")
	if !result.Verified {
		t.Fatalf("unexpected result %#v", result)
	}
	if !bundleCalled {
		t.Fatal("expected bundle verifier to be called")
	}
}

func TestVerifyImageUsesConfiguredSigners(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()

	parseReference = func(string, ...name.Option) (name.Reference, error) {
		return name.MustParseReference("ghcr.io/example/demo:latest"), nil
	}
	trustedRootFunc = func() (root.TrustedMaterial, error) {
		return nil, nil
	}
	verifyImageAttestations = func(_ context.Context, _ name.Reference, opts *cosign.CheckOpts, _ ...name.Option) ([]cosignoci.Signature, bool, error) {
		if len(opts.Identities) != 1 || opts.Identities[0].Issuer != "https://issuer.example.com" || opts.Identities[0].Subject != "signer@example.com" {
			t.Fatalf("unexpected identities %#v", opts.Identities)
		}
		return nil, false, nil
	}
	verifyImageSignatures = func(_ context.Context, _ name.Reference, _ *cosign.CheckOpts) ([]cosignoci.Signature, bool, error) {
		t.Fatal("did not expect legacy signature verification when bundle verification succeeds")
		return nil, false, nil
	}

	result := VerifyImageWithSigners(context.Background(), "ghcr.io/example/demo:latest", []TrustedSigner{{Issuer: "https://issuer.example.com", Subject: "signer@example.com"}})
	if !result.Verified {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestVerifyImageEnablesOCI11Verification(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()

	parseReference = func(string, ...name.Option) (name.Reference, error) {
		return name.MustParseReference("example.com/demo/app:latest"), nil
	}
	trustedRootFunc = func() (root.TrustedMaterial, error) {
		return nil, nil
	}
	verifyImageAttestations = func(_ context.Context, _ name.Reference, _ *cosign.CheckOpts, _ ...name.Option) ([]cosignoci.Signature, bool, error) {
		return nil, false, &cosign.ErrNoMatchingAttestations{}
	}
	verifyImageSignatures = func(_ context.Context, _ name.Reference, opts *cosign.CheckOpts) ([]cosignoci.Signature, bool, error) {
		if opts == nil {
			t.Fatal("expected check options")
		}
		if !opts.ExperimentalOCI11 {
			t.Fatal("expected OCI 1.1 verification to be enabled")
		}
		if len(opts.Identities) != 1 || opts.Identities[0].IssuerRegExp != ".*" || opts.Identities[0].SubjectRegExp != ".*" {
			t.Fatalf("unexpected identities %#v", opts.Identities)
		}
		return nil, false, nil
	}

	result := VerifyImage(context.Background(), "example.com/demo/app:latest")
	if !result.Verified {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestVerifyImageReturnsReasonOnUnsignedImage(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()

	parseReference = func(string, ...name.Option) (name.Reference, error) {
		return name.MustParseReference("example.com/demo/app:latest"), nil
	}
	trustedRootFunc = func() (root.TrustedMaterial, error) {
		return nil, nil
	}
	verifyImageAttestations = func(context.Context, name.Reference, *cosign.CheckOpts, ...name.Option) ([]cosignoci.Signature, bool, error) {
		return nil, false, &cosign.ErrNoMatchingAttestations{}
	}
	verifyImageSignatures = func(context.Context, name.Reference, *cosign.CheckOpts) ([]cosignoci.Signature, bool, error) {
		return nil, false, &cosign.ErrNoSignaturesFound{}
	}

	result := VerifyImage(context.Background(), "example.com/demo/app:latest")
	if result.Verified || !strings.Contains(result.Reason, "no signatures") {
		t.Fatalf("unexpected result %#v", result)
	}
}

func stubVerifierDeps(t *testing.T) func() {
	t.Helper()
	origParse := parseReference
	origTrustedRoot := trustedRootFunc
	origVerifyAttestations := verifyImageAttestations
	origVerify := verifyImageSignatures
	resetTrustRootCache()
	return func() {
		parseReference = origParse
		trustedRootFunc = origTrustedRoot
		verifyImageAttestations = origVerifyAttestations
		verifyImageSignatures = origVerify
		resetTrustRootCache()
	}
}
