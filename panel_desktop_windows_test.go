//go:build desktop && windows

package main

import "testing"

func TestDesktopWindowSize(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		dpi, workWidth, workHeight int
		wantWidth, wantHeight      int
	}{
		{"100 percent", 96, 1920, 1040, 1280, 860},
		{"150 percent", 144, 2560, 1400, 1920, 1290},
		{"200 percent", 192, 3840, 2080, 2560, 1720},
		{"small screen", 144, 1920, 1040, 1872, 992},
	} {
		t.Run(tc.name, func(t *testing.T) {
			width, height := desktopWindowSize(tc.dpi, tc.workWidth, tc.workHeight)
			if width != tc.wantWidth || height != tc.wantHeight {
				t.Fatalf("size = %dx%d, want %dx%d", width, height, tc.wantWidth, tc.wantHeight)
			}
		})
	}
}
