package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPutProfilePersistsSavedProfile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")
	if err := os.WriteFile(defaultPath, []byte("WFB_WEB_PROFILE=gs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(cfgPath, defaultPath, "", "gs")
	req := httptest.NewRequest(http.MethodPut, "/api/profile", strings.NewReader(`{"profile":"drone"}`))
	rec := httptest.NewRecorder()

	server.putProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "WFB_WEB_PROFILE=drone # saved wfb-web profile") {
		t.Fatalf("profile was not persisted:\n%s", got)
	}
}
