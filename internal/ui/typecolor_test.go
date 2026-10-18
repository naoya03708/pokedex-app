package ui

import (
	"math"
	"testing"

	mui "github.com/egoist/mygo/ui"
)

// The helpers below compute WCAG 2.x contrast independently of the code
// under test, so the palette is checked against the standard formula.

// linear converts an 8-bit sRGB channel to linear light.
func linear(c uint8) float64 {
	v := float64(c) / 255
	if v <= 0.03928 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func luminance(c mui.Color) float64 {
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}

func contrast(a, b mui.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func TestTypeStyle_FR2_EveryKnownTypeHasReadableText(t *testing.T) {
	if len(knownTypes) != 18 {
		t.Fatalf("knownTypes = %d, want the 18 types", len(knownTypes))
	}
	for _, name := range append([]string{"no-such-type"}, knownTypes...) {
		s := styleOf(name)

		if c := contrast(s.Bg, s.Fg); c < 4.5 {
			t.Errorf("%s: contrast %.2f:1 between badge and text, want >= 4.5:1", name, c)
		}
	}
}

func TestTypeStyle_FR2_EachKnownTypeHasItsOwnColor(t *testing.T) {
	seen := map[mui.Color]string{}
	for _, name := range knownTypes {
		bg := styleOf(name).Bg
		if other, dup := seen[bg]; dup {
			t.Errorf("%s and %s share a color", name, other)
		}
		seen[bg] = name
	}
}

func TestTypeStyle_FR2_UnknownTypeIsNeutralNotAKnownColor(t *testing.T) {
	neutral := styleOf("no-such-type")
	if neutral.Bg == (mui.Color{}) {
		t.Fatal("unknown type has no color at all")
	}
	for _, name := range knownTypes {
		if styleOf(name).Bg == neutral.Bg {
			t.Errorf("%s has the neutral color", name)
		}
	}
}

func TestTypeStyle_FR2_NamesAreMatchedCaseInsensitively(t *testing.T) {
	if styleOf("Fire") != styleOf("fire") {
		t.Error("Fire and fire differ")
	}
}
