package bridge

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenHandlerRequiresValidToken(t *testing.T) {
	t.Parallel()
	session := &Session{ID: "session", Token: "secret", Port: 1234, StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	handler := openHandler(session, "container", func(string) error { return nil })
	req := httptest.NewRequest(http.MethodPost, "/open", nil)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status %d", res.Code)
	}
}

func TestOpenHandlerForwardsURL(t *testing.T) {
	t.Parallel()
	statusPath := filepath.Join(t.TempDir(), "bridge-status.json")
	session := &Session{ID: "session", Token: "secret", Port: 1234, StatusPath: statusPath}
	var opened string
	handler := openHandler(session, "container", func(target string) error {
		opened = target
		return nil
	})
	req := httptest.NewRequest(http.MethodPost, "/open?url=https%3A%2F%2Fexample.com&token=secret", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("unexpected status %d", res.Code)
	}
	if opened != "https://example.com" {
		t.Fatalf("unexpected opened url %q", opened)
	}
	if _, err := os.Stat(statusPath); err != nil {
		t.Fatalf("expected status file: %v", err)
	}
}
