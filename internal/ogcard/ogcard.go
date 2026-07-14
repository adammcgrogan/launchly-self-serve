// Package ogcard renders a 1200x630 social share image (og:image) for a site
// from its business name, tagline, location, and accent colour. The card is a
// PNG — Facebook and WhatsApp don't render SVG og:images — drawn server-side
// with the vector Go fonts bundled in golang.org/x/image, so there's no font
// file to vendor or ship.
package ogcard

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	width   = 1200
	height  = 630
	marginX = 90
)

var (
	boldFont    *opentype.Font
	regularFont *opentype.Font
)

func init() {
	var err error
	if boldFont, err = opentype.Parse(gobold.TTF); err != nil {
		panic("ogcard: parse bold font: " + err.Error())
	}
	if regularFont, err = opentype.Parse(goregular.TTF); err != nil {
		panic("ogcard: parse regular font: " + err.Error())
	}
}

// Card is the content to render onto the share image.
type Card struct {
	BusinessName string
	Tagline      string
	Location     string
	Footer       string // e.g. "acme.launchly.ltd"
	AccentHex    string // "#RRGGBB"; falls back to indigo if empty/invalid
}

// Render draws the card and returns the encoded PNG bytes.
func Render(c Card) ([]byte, error) {
	accent := parseHex(c.AccentHex, color.RGBA{0x4F, 0x46, 0xE5, 0xFF})
	ink := readableInk(accent)
	muted := blend(ink, accent, 0.28) // ink softened toward the background

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{accent}, image.Point{}, draw.Src)

	// A thin accent rule top-left as a small brand flourish.
	draw.Draw(img, image.Rect(marginX, 96, marginX+90, 104), &image.Uniform{ink}, image.Point{}, draw.Src)

	maxTextWidth := width - 2*marginX

	// Business name: shrink the point size until it fits in at most two lines.
	name := strings.TrimSpace(c.BusinessName)
	if name == "" {
		name = "Untitled site"
	}
	var nameLines []string
	nameSize := 92.0
	for nameSize >= 52 {
		face := newFace(boldFont, nameSize)
		nameLines = wrap(face, name, maxTextWidth)
		face.Close()
		if len(nameLines) <= 2 {
			break
		}
		nameSize -= 6
	}
	if len(nameLines) > 2 {
		nameLines = nameLines[:2]
	}

	// Everything below the accent rule flows top-down so a two-line name never
	// collides with the tagline or location; the footer is pinned to the base.
	y := 130
	nameFace := newFace(boldFont, nameSize)
	nameLineH := int(nameSize * 1.14)
	for _, line := range nameLines {
		y += nameLineH
		drawText(img, nameFace, ink, marginX, y, line)
	}
	nameFace.Close()

	// Tagline: up to two lines in the muted ink.
	if tagline := strings.TrimSpace(c.Tagline); tagline != "" {
		tagFace := newFace(regularFont, 40)
		lines := wrap(tagFace, tagline, maxTextWidth)
		if len(lines) > 2 {
			lines = lines[:2]
		}
		y += 26
		for _, line := range lines {
			y += 50
			drawText(img, tagFace, muted, marginX, y, line)
		}
		tagFace.Close()
	}

	// Location badge: a small filled dot + text, following the flow.
	if loc := strings.TrimSpace(c.Location); loc != "" {
		locFace := newFace(regularFont, 34)
		y += 44
		drawDot(img, ink, marginX+7, y-11, 7)
		drawText(img, locFace, ink, marginX+30, y, loc)
		locFace.Close()
	}

	// Footer: the site URL (left) and a Launchly credit (right).
	footFace := newFace(regularFont, 30)
	fy := height - 60
	if footer := strings.TrimSpace(c.Footer); footer != "" {
		drawText(img, footFace, muted, marginX, fy, footer)
	}
	credit := "Built with Launchly"
	cw := textWidth(footFace, credit)
	drawText(img, footFace, muted, width-marginX-cw, fy, credit)
	footFace.Close()

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func newFace(f *opentype.Font, size float64) font.Face {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		// Sizes and fonts are fixed constants, so this can't fail in practice.
		panic("ogcard: new face: " + err.Error())
	}
	return face
}

func drawText(img draw.Image, face font.Face, c color.Color, x, baseline int, s string) {
	d := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{c},
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(s)
}

func textWidth(face font.Face, s string) int {
	return font.MeasureString(face, s).Round()
}

// wrap greedily splits s into lines that each fit within maxWidth px.
func wrap(face font.Face, s string, maxWidth int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if textWidth(face, candidate) <= maxWidth {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = word
	}
	return append(lines, current)
}

func drawDot(img draw.Image, c color.Color, cx, cy, r int) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				img.Set(cx+dx, cy+dy, c)
			}
		}
	}
}

// readableInk returns black or white, whichever contrasts better with bg,
// using the same relative-luminance rule as the rest of the app.
func readableInk(bg color.RGBA) color.RGBA {
	lum := 0.299*float64(bg.R) + 0.587*float64(bg.G) + 0.114*float64(bg.B)
	if lum > 150 {
		return color.RGBA{0x11, 0x11, 0x11, 0xFF}
	}
	return color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
}

// blend mixes fg toward bg by t (0 = fg, 1 = bg).
func blend(fg, bg color.RGBA, t float64) color.RGBA {
	mix := func(a, b uint8) uint8 { return uint8(float64(a)*(1-t) + float64(b)*t) }
	return color.RGBA{mix(fg.R, bg.R), mix(fg.G, bg.G), mix(fg.B, bg.B), 0xFF}
}

func parseHex(s string, fallback color.RGBA) color.RGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return fallback
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return fallback
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xFF}
}
