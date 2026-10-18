package ui

import (
	"sort"
	"strings"

	mui "github.com/egoist/mygo/ui"
)

// typeStyle is the badge background and its text color for a Pokémon type.
type typeStyle struct{ Bg, Fg mui.Color }

var (
	ink   = mui.Hex("#1b1b1f")
	white = mui.Hex("#ffffff")
)

// palette holds one color per type. The text color is whichever of ink and
// white reads better on the background; every pair has a contrast of at
// least 4.5:1, which typecolor_test.go checks.
var palette = map[string]typeStyle{
	"normal":   {mui.Hex("#a8a77a"), ink},
	"fire":     {mui.Hex("#ee8130"), ink},
	"water":    {mui.Hex("#6390f0"), ink},
	"electric": {mui.Hex("#f7d02c"), ink},
	"grass":    {mui.Hex("#7ac74c"), ink},
	"ice":      {mui.Hex("#96d9d6"), ink},
	"fighting": {mui.Hex("#c22e28"), white},
	"poison":   {mui.Hex("#a33ea1"), white},
	"ground":   {mui.Hex("#e2bf65"), ink},
	"flying":   {mui.Hex("#a98ff3"), ink},
	"psychic":  {mui.Hex("#f95587"), ink},
	"bug":      {mui.Hex("#a6b91a"), ink},
	"rock":     {mui.Hex("#b6a136"), ink},
	"ghost":    {mui.Hex("#735797"), white},
	"dragon":   {mui.Hex("#6f35fc"), white},
	"dark":     {mui.Hex("#705746"), white},
	"steel":    {mui.Hex("#b7b7ce"), ink},
	"fairy":    {mui.Hex("#d685ad"), ink},
}

// neutral is the style of a type this app does not know, such as one a
// later game adds.
var neutral = typeStyle{mui.Hex("#9a9aa2"), ink}

// knownTypes lists the types that have their own color, sorted.
var knownTypes = func() []string {
	names := make([]string, 0, len(palette))
	for n := range palette {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}()

// styleOf returns the style of a type name, ignoring case.
func styleOf(name string) typeStyle {
	if s, ok := palette[strings.ToLower(name)]; ok {
		return s
	}
	return neutral
}
