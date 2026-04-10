package security

import (
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sigstore/cosign/v3/pkg/cosign"
)

type TrustedSigner struct {
	Issuer        string `toml:"issuer"`
	Subject       string `toml:"subject"`
	IssuerRegExp  string `toml:"issuer_regexp"`
	SubjectRegExp string `toml:"subject_regexp"`
}

func signerIdentities(ref name.Reference, configured []TrustedSigner) []cosign.Identity {
	if len(configured) > 0 {
		return toCosignIdentities(configured)
	}
	if recommended := recommendedTrustedSigners(ref); len(recommended) > 0 {
		return toCosignIdentities(recommended)
	}
	return []cosign.Identity{{IssuerRegExp: ".*", SubjectRegExp: ".*"}}
}

func toCosignIdentities(signers []TrustedSigner) []cosign.Identity {
	identities := make([]cosign.Identity, 0, len(signers))
	for _, signer := range signers {
		identities = append(identities, cosign.Identity{
			Issuer:        signer.Issuer,
			Subject:       signer.Subject,
			IssuerRegExp:  signer.IssuerRegExp,
			SubjectRegExp: signer.SubjectRegExp,
		})
	}
	return identities
}

func recommendedTrustedSigners(ref name.Reference) []TrustedSigner {
	repo := ref.Context()
	if repo.Registry.RegistryStr() != "ghcr.io" {
		return nil
	}
	parts := strings.Split(repo.RepositoryStr(), "/")
	if len(parts) < 2 {
		return nil
	}
	githubRepo := regexp.QuoteMeta(parts[0] + "/" + parts[1])
	return []TrustedSigner{{
		Issuer:        "https://token.actions.githubusercontent.com",
		SubjectRegExp: "^https://github.com/" + githubRepo + "/.github/workflows/.+@refs/.+$",
	}}
}
