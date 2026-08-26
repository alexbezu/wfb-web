package profile

import "testing"

func TestDetectUsesSavedProfileFirst(t *testing.T) {
	selection := Detect("drone", "gs")
	if selection.Profile != "drone" {
		t.Fatalf("expected saved drone profile, got %q", selection.Profile)
	}
	if selection.Source != "saved" {
		t.Fatalf("expected saved source, got %q", selection.Source)
	}
}
