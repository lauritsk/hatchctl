package reconcile

import (
	"testing"

	"github.com/lauritsk/hatchctl/internal/security"
)

func TestWithTrustedSignersReturnsIsolatedExecutorClone(t *testing.T) {
	t.Parallel()

	base := &Executor{}
	one := base.WithTrustedSigners([]security.TrustedSigner{{Issuer: "https://issuer.one"}})
	two := base.WithTrustedSigners([]security.TrustedSigner{{Issuer: "https://issuer.two"}})

	if len(base.trustedSigners) != 0 {
		t.Fatalf("expected base executor to remain unchanged, got %#v", base.trustedSigners)
	}
	if got := len(one.trustedSigners); got != 1 || one.trustedSigners[0].Issuer != "https://issuer.one" {
		t.Fatalf("unexpected first clone signers %#v", one.trustedSigners)
	}
	if got := len(two.trustedSigners); got != 1 || two.trustedSigners[0].Issuer != "https://issuer.two" {
		t.Fatalf("unexpected second clone signers %#v", two.trustedSigners)
	}
}
