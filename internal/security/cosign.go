package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	cosignoci "github.com/sigstore/cosign/v3/pkg/oci"
	"github.com/sigstore/sigstore-go/pkg/root"
)

const (
	CosignStrictEnvVar          = "HATCHCTL_COSIGN_STRICT"
	AllowInsecureFeaturesEnvVar = "HATCHCTL_ALLOW_INSECURE_FEATURES"
)

var (
	parseReference        = name.ParseReference
	trustedRootFunc       = cosign.TrustedRoot
	verifyImageSignatures = cosign.VerifyImageSignatures

	trustedRootOnce sync.Once
	trustedRootVal  root.TrustedMaterial
	trustedRootErr  error
)

type VerificationResult struct {
	Ref      string
	Verified bool
	Reason   string
}

func VerifyImage(ctx context.Context, ref string) VerificationResult {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return VerificationResult{Verified: true}
	}
	parsedRef, err := parseReference(ref)
	if err != nil {
		return VerificationResult{Ref: ref, Reason: fmt.Sprintf("unsupported image reference: %v", err)}
	}
	trustedMaterial, err := trustedMaterial()
	if err != nil {
		return VerificationResult{Ref: ref, Reason: fmt.Sprintf("load sigstore trusted root: %v", err)}
	}
	_, _, err = verifyImageSignatures(ctx, parsedRef, &cosign.CheckOpts{
		TrustedMaterial: trustedMaterial,
		ClaimVerifier:   cosign.SimpleClaimVerifier,
	})
	if err == nil {
		return VerificationResult{Ref: ref, Verified: true}
	}
	var noSigs *cosign.ErrNoSignaturesFound
	var noMatch *cosign.ErrNoMatchingSignatures
	if errors.As(err, &noSigs) || errors.As(err, &noMatch) {
		reason := "no matching signatures found"
		if errors.As(err, &noSigs) {
			reason = "no signatures found"
		}
		return VerificationResult{Ref: ref, Reason: reason}
	}
	return VerificationResult{Ref: ref, Reason: fmt.Sprintf("verification failed: %v", err)}
}

func trustedMaterial() (root.TrustedMaterial, error) {
	trustedRootOnce.Do(func() {
		trustedRootVal, trustedRootErr = trustedRootFunc()
	})
	return trustedRootVal, trustedRootErr
}

func (r VerificationResult) Error() string {
	if r.Ref == "" {
		return r.Reason
	}
	return fmt.Sprintf("unable to verify %s: %s", r.Ref, r.Reason)
}

func resetTrustRootCache() {
	trustedRootOnce = sync.Once{}
	trustedRootVal = nil
	trustedRootErr = nil
}

var _ []cosignoci.Signature
