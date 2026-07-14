package ogcard

import (
	"bytes"
	"image/png"
	"testing"
)

// TestWrapAndMeasure guards the text-width calculation: a realistic business
// name at the largest heading size must not fit on one line (regression for a
// double-divide bug that reported every string as a few pixels wide, so
// nothing ever wrapped or shrank and long names ran off the card).
func TestWrapAndMeasure(t *testing.T) {
	face := newFace(boldFont, 92)
	defer face.Close()

	long := "Metropolitan Plumbing & Heating Solutions"
	if w := textWidth(face, long); w < 1020 {
		t.Fatalf("textWidth of a long name at 92px = %d, expected it to exceed the card's text area (1020px)", w)
	}
	if lines := wrap(face, long, 1020); len(lines) < 2 {
		t.Fatalf("wrap(%q) produced %d line(s), want at least 2", long, len(lines))
	}
	if lines := wrap(face, "Acme Co", 1020); len(lines) != 1 {
		t.Fatalf("wrap of a short name produced %d lines, want 1", len(lines))
	}
}

func TestRenderProducesValidPNG(t *testing.T) {
	cases := []Card{
		{BusinessName: "Belfast Boiler Care", Tagline: "Heating & plumbing you can rely on", Location: "Belfast, NI", Footer: "belfast-boilers.launchly.ltd", AccentHex: "#B45309"},
		{BusinessName: "A really long business name that will not fit on a single line at the largest size", AccentHex: ""},
		{BusinessName: "", AccentHex: "not-a-hex"}, // empty name + bad colour must still render
	}
	for i, c := range cases {
		data, err := Render(c)
		if err != nil {
			t.Fatalf("case %d: Render returned error: %v", i, err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("case %d: output is not a valid PNG: %v", i, err)
		}
		if b := img.Bounds(); b.Dx() != width || b.Dy() != height {
			t.Fatalf("case %d: got %dx%d, want %dx%d", i, b.Dx(), b.Dy(), width, height)
		}
	}
}
