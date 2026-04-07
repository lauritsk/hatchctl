package security

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	cosignoci "github.com/sigstore/cosign/v3/pkg/oci"
	"github.com/sigstore/sigstore-go/pkg/root"
)

const CosignStrictEnvVar = "HATCHCTL_COSIGN_STRICT"

var (
	warningWriter         io.Writer = os.Stderr
	parseReference                  = name.ParseReference
	trustedRootFunc                 = cosign.TrustedRoot
	verifyImageSignatures           = cosign.VerifyImageSignatures

	trustedRootOnce sync.Once
	trustedRootVal  root.TrustedMaterial
	trustedRootErr  error
)

func VerifyImage(ctx context.Context, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	parsedRef, err := parseReference(ref)
	if err != nil {
		return handleVerifyFailure(ref, fmt.Sprintf("unsupported image reference: %v", err))
	}
	trustedMaterial, err := trustedMaterial()
	if err != nil {
		return handleVerifyFailure(ref, fmt.Sprintf("load sigstore trusted root: %v", err))
	}
	_, _, err = verifyImageSignatures(ctx, parsedRef, &cosign.CheckOpts{
		TrustedMaterial: trustedMaterial,
		ClaimVerifier:   cosign.SimpleClaimVerifier,
	})
	if err == nil {
		return nil
	}
	var noSigs *cosign.ErrNoSignaturesFound
	var noMatch *cosign.ErrNoMatchingSignatures
	if errors.As(err, &noSigs) || errors.As(err, &noMatch) {
		reason := "no matching signatures found"
		if errors.As(err, &noSigs) {
			reason = "no signatures found"
		}
		return handleVerifyFailure(ref, reason)
	}
	return handleVerifyFailure(ref, fmt.Sprintf("verification failed: %v", err))
}

func trustedMaterial() (root.TrustedMaterial, error) {
	trustedRootOnce.Do(func() {
		trustedRootVal, trustedRootErr = trustedRootFunc()
	})
	return trustedRootVal, trustedRootErr
}

func handleVerifyFailure(ref string, reason string) error {
	message := fmt.Sprintf("warning: unable to verify %s: %s", ref, reason)
	if cosignStrict() {
		return fmt.Errorf("unable to verify %s: %s", ref, reason)
	}
	_, _ = fmt.Fprintln(warningWriter, message)
	return nil
}

func cosignStrict() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(CosignStrictEnvVar))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func resetTrustRootCache() {
	trustedRootOnce = sync.Once{}
	trustedRootVal = nil
	trustedRootErr = nil
}

var _ []cosignoci.Signature
