package tray

import (
	"testing"
)

func TestIconBytes(t *testing.T) {
	icon := getIconBytes()
	if len(icon) < 22 {
		t.Fatalf("Icon bytes too short: %d", len(icon))
	}
	// Check if valid ICO header
	if icon[0] != 0 || icon[1] != 0 || icon[2] != 1 || icon[3] != 0 {
		t.Errorf("Not a valid ICO header: %v", icon[:4])
	}
}
