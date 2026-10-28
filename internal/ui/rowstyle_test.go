package ui

import (
	"testing"

	mui "github.com/egoist/mygo/ui"
)

func TestRowNumberColor_FR1_SelectedRowNumberMatchesTheNameColorInBothAppearances(t *testing.T) {
	// The list draws the name of the selected row in the theme's AccentText,
	// so the number must use it too, not the muted gray, which was nearly
	// invisible on the selection color in the screenshots.
	for name, th := range map[string]*mui.Theme{"light": mui.LightTheme(), "dark": mui.DarkTheme()} {
		if got := rowNumberColor(th, true); got != th.AccentText {
			t.Errorf("%s: got %+v, want the accent text color %+v", name, got, th.AccentText)
		}
		if got := rowNumberColor(th, true); got == th.TextMuted {
			t.Errorf("%s: the selected row still has the muted number color", name)
		}
	}
}

func TestRowNumberColor_FR1_OtherRowsKeepTheMutedColor(t *testing.T) {
	for name, th := range map[string]*mui.Theme{"light": mui.LightTheme(), "dark": mui.DarkTheme()} {
		if got := rowNumberColor(th, false); got != th.TextMuted {
			t.Errorf("%s: got %+v, want the muted text color %+v", name, got, th.TextMuted)
		}
	}
}

// The longest number is a form such as "#10196": six characters. A digit in
// a proportional UI font is about 0.62 em wide, and "#" is about as wide;
// 0.65 em leaves a margin. ui.NewTester cannot show wrapping (it does not
// use the real font), so the width is checked against that rule instead.
func TestNumberColumn_FR1_FitsASixCharacterNumberOnOneLine(t *testing.T) {
	for name, th := range map[string]*mui.Theme{"light": mui.LightTheme(), "dark": mui.DarkTheme()} {
		need := 6 * 0.65 * th.FontSize

		if float32(numberColumnWidth) < need {
			t.Errorf("%s: the number column is %d wide, \"#10196\" needs about %.0f at font size %.0f",
				name, numberColumnWidth, need, th.FontSize)
		}
	}
}
