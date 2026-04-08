package runtime

import "testing"

func TestRedactSensitiveMapRedactsSensitiveKeys(t *testing.T) {
	values := map[string]string{
		"API_TOKEN": "secret",
		"PATH":      "/usr/bin",
	}
	redacted := redactSensitiveMap(values)
	if redacted["API_TOKEN"] != redactedValue {
		t.Fatalf("expected API_TOKEN to be redacted, got %q", redacted["API_TOKEN"])
	}
	if redacted["PATH"] != "/usr/bin" {
		t.Fatalf("expected PATH to be preserved, got %q", redacted["PATH"])
	}
}
