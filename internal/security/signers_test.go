package security

import (
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
)

func TestSignerIdentitiesRecommendGitHubActionsForGHCRRepositories(t *testing.T) {
	ref := name.MustParseReference("ghcr.io/lauritsk/devcontainer:latest")
	identities := signerIdentities(ref, nil)
	if len(identities) != 1 {
		t.Fatalf("unexpected identities %#v", identities)
	}
	if identities[0].Issuer != "https://token.actions.githubusercontent.com" {
		t.Fatalf("unexpected identity %#v", identities[0])
	}
	if !strings.Contains(identities[0].SubjectRegExp, "github.com/lauritsk/devcontainer") {
		t.Fatalf("unexpected identity %#v", identities[0])
	}
}

func TestSignerIdentitiesFallbackToWildcardWhenNoRecommendationExists(t *testing.T) {
	ref := name.MustParseReference("docker.io/library/alpine:latest")
	identities := signerIdentities(ref, nil)
	if len(identities) != 1 || identities[0].IssuerRegExp != ".*" || identities[0].SubjectRegExp != ".*" {
		t.Fatalf("unexpected identities %#v", identities)
	}
}
