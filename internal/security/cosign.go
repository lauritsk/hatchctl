package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	cosignoci "github.com/sigstore/cosign/v3/pkg/oci"
	ociremote "github.com/sigstore/cosign/v3/pkg/oci/remote"
	"github.com/sigstore/sigstore-go/pkg/root"
)

const (
	CosignStrictEnvVar          = "HATCHCTL_COSIGN_STRICT"
	AllowInsecureFeaturesEnvVar = "HATCHCTL_ALLOW_INSECURE_FEATURES"
)

var (
	parseReference          = name.ParseReference
	trustedRootFunc         = cosign.TrustedRoot
	verifyImageAttestations = cosign.VerifyImageAttestations
	verifyImageSignatures   = cosign.VerifyImageSignatures

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
	return VerifyImageWithSigners(ctx, ref, nil)
}

func VerifyImageWithSigners(ctx context.Context, ref string, signers []TrustedSigner) VerificationResult {
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
	identities := signerIdentities(parsedRef, signers)
	registryOpts := registryClientOptions(ctx)
	_, _, err = verifyImageAttestations(ctx, parsedRef, bundleCheckOpts(trustedMaterial, identities, registryOpts))
	if err == nil {
		return VerificationResult{Ref: ref, Verified: true}
	}
	bundleErr := err
	_, _, err = verifyImageSignatures(ctx, parsedRef, signatureCheckOpts(trustedMaterial, identities, registryOpts))
	if err == nil {
		return VerificationResult{Ref: ref, Verified: true}
	}
	if !isMissingSignatureMaterial(bundleErr) {
		return verificationFailure(ref, bundleErr)
	}
	return signatureVerificationFailure(ref, err)
}

func registryClientOptions(ctx context.Context) []ociremote.Option {
	return []ociremote.Option{ociremote.WithRemoteOptions(
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	)}
}

func bundleCheckOpts(trustedMaterial root.TrustedMaterial, identities []cosign.Identity, registryOpts []ociremote.Option) *cosign.CheckOpts {
	return &cosign.CheckOpts{
		TrustedMaterial:    trustedMaterial,
		ClaimVerifier:      cosign.IntotoSubjectClaimVerifier,
		Identities:         identities,
		NewBundleFormat:    true,
		RegistryClientOpts: registryOpts,
	}
}

func signatureCheckOpts(trustedMaterial root.TrustedMaterial, identities []cosign.Identity, registryOpts []ociremote.Option) *cosign.CheckOpts {
	return &cosign.CheckOpts{
		TrustedMaterial:    trustedMaterial,
		ClaimVerifier:      cosign.SimpleClaimVerifier,
		Identities:         identities,
		ExperimentalOCI11:  true,
		RegistryClientOpts: registryOpts,
	}
}

func verificationFailure(ref string, err error) VerificationResult {
	return VerificationResult{Ref: ref, Reason: fmt.Sprintf("verification failed: %v", err)}
}

func signatureVerificationFailure(ref string, err error) VerificationResult {
	var noSigs *cosign.ErrNoSignaturesFound
	var noMatch *cosign.ErrNoMatchingSignatures
	if errors.As(err, &noSigs) || errors.As(err, &noMatch) {
		reason := "no matching signatures found"
		if errors.As(err, &noSigs) {
			reason = "no signatures found"
		}
		return VerificationResult{Ref: ref, Reason: reason}
	}
	return verificationFailure(ref, err)
}

func isMissingSignatureMaterial(err error) bool {
	if err == nil {
		return false
	}
	var noAttestations *cosign.ErrNoMatchingAttestations
	var noSignatures *cosign.ErrNoSignaturesFound
	var noMatchingSignatures *cosign.ErrNoMatchingSignatures
	return errors.As(err, &noAttestations) || errors.As(err, &noSignatures) || errors.As(err, &noMatchingSignatures)
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
