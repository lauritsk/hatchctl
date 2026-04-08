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

func TestVerifyImageUsesCosignVerifierWhenAvailable(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()

	parseReference = func(string, ...name.Option) (name.Reference, error) {
		return name.MustParseReference("example.com/demo/app:latest"), nil
	}
	trustedRootFunc = func() (root.TrustedMaterial, error) {
		return nil, nil
	}
	called := false
	verifyImageSignatures = func(context.Context, name.Reference, *cosign.CheckOpts) ([]cosignoci.Signature, bool, error) {
		called = true
		return nil, false, nil
	}

	result := VerifyImage(context.Background(), "example.com/demo/app:latest")
	if !result.Verified {
		t.Fatalf("unexpected result %#v", result)
	}
	if !called {
		t.Fatal("expected cosign verifier to be called")
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
	origVerify := verifyImageSignatures
	resetTrustRootCache()
	return func() {
		parseReference = origParse
		trustedRootFunc = origTrustedRoot
		verifyImageSignatures = origVerify
		resetTrustRootCache()
	}
}
