package security

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	cosignoci "github.com/sigstore/cosign/v3/pkg/oci"
	"github.com/sigstore/sigstore-go/pkg/root"
)

func TestVerifyImageWarnsWhenReferenceCannotBeParsed(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()

	var warnings bytes.Buffer
	warningWriter = &warnings

	if err := VerifyImage(context.Background(), "not a ref%%"); err != nil {
		t.Fatalf("verify image: %v", err)
	}
	if got := warnings.String(); !strings.Contains(got, "unsupported image reference") {
		t.Fatalf("unexpected warning %q", got)
	}
}

func TestVerifyImageReturnsErrorInStrictMode(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()
	t.Setenv(CosignStrictEnvVar, "1")

	parseReference = func(string, ...name.Option) (name.Reference, error) {
		return nil, errors.New("bad ref")
	}
	if err := VerifyImage(context.Background(), "example.com/app:latest"); err == nil || !strings.Contains(err.Error(), "bad ref") {
		t.Fatalf("expected strict verification error, got %v", err)
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

	if err := VerifyImage(context.Background(), "example.com/demo/app:latest"); err != nil {
		t.Fatalf("verify image: %v", err)
	}
	if !called {
		t.Fatal("expected cosign verifier to be called")
	}
}

func TestVerifyImageWarnsOnUnsignedImagesByDefault(t *testing.T) {
	restore := stubVerifierDeps(t)
	defer restore()

	var warnings bytes.Buffer
	warningWriter = &warnings
	parseReference = func(string, ...name.Option) (name.Reference, error) {
		return name.MustParseReference("example.com/demo/app:latest"), nil
	}
	trustedRootFunc = func() (root.TrustedMaterial, error) {
		return nil, nil
	}
	verifyImageSignatures = func(context.Context, name.Reference, *cosign.CheckOpts) ([]cosignoci.Signature, bool, error) {
		return nil, false, &cosign.ErrNoSignaturesFound{}
	}

	if err := VerifyImage(context.Background(), "example.com/demo/app:latest"); err != nil {
		t.Fatalf("verify image: %v", err)
	}
	if got := warnings.String(); !strings.Contains(got, "no signatures") {
		t.Fatalf("unexpected warning %q", got)
	}
}

func stubVerifierDeps(t *testing.T) func() {
	t.Helper()
	origWarningWriter := warningWriter
	origParse := parseReference
	origTrustedRoot := trustedRootFunc
	origVerify := verifyImageSignatures
	resetTrustRootCache()
	return func() {
		warningWriter = origWarningWriter
		parseReference = origParse
		trustedRootFunc = origTrustedRoot
		verifyImageSignatures = origVerify
		resetTrustRootCache()
	}
}
