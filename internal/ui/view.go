// Package ui builds the window's interface from an app.Model.
package ui

import (
	"fmt"
	"strings"

	mui "github.com/egoist/mygo/ui"

	"github.com/naoya03708/pokedex-app/internal/app"
	"github.com/naoya03708/pokedex-app/internal/pokeapi"
)

const (
	maxBaseStat = 255 // the highest base stat, so a bar is value/maxBaseStat
	spriteSize  = 128

	// numberColumnWidth is the width of the Pokédex number in a result row. It
	// holds "#10196", the longest number (a form), on one line.
	numberColumnWidth = 56

	// thumbSize is the side of a result row's sprite. The slot has this size
	// whether or not there is an image, so rows do not change with it.
	thumbSize = 40
	// maxBitmaps bounds the decoded row sprites kept in memory (about 17 KB
	// each at 64x64 pixels, but a long scroll would otherwise keep them all).
	maxBitmaps = 300
)

// View draws the search box, the result list and the detail pane. Its fields
// are widget state that lasts between frames: never an element or context.
type View struct {
	m *app.Model

	split  float32 // width of the result list pane (DIPs)
	list   mui.ListState
	cursor int // highlighted row of the list, -1 for none

	// The cursor follows the selection when the visible rows change.
	synced      bool
	syncedQuery string
	syncedCount int

	// The decoded sprite of bitmapOf, made once rather than in every frame.
	bitmap   *mui.Bitmap
	bitmapOf string

	// The decoded row sprites by Pokédex number. A nil value is an image that
	// does not decode.
	thumbs map[int]*mui.Bitmap
}

// New returns a View of m.
func New(m *app.Model) *View {
	return &View{m: m, split: 260, cursor: -1, thumbs: map[int]*mui.Bitmap{}}
}

// Build is the window's view function. It runs on the UI thread.
func (v *View) Build(c *mui.Context) {
	v.syncCursor()
	mui.Column(c).Fill().Padding(12).Gap(10).Children(func() {
		mui.SearchField(c, &v.m.Query).Label("Search Pokémon")
		mui.Split(c, &v.split, func() { v.results(c) }, func() { v.detail(c) }).Grow(1)
	})
}

// syncCursor puts the cursor on the selected Pokémon whenever the rows
// change (a new query, or the list arriving), or on nothing if it is hidden.
func (v *View) syncCursor() {
	vis := v.m.Visible()
	if v.synced && v.syncedQuery == v.m.Query && v.syncedCount == len(vis) {
		return
	}
	v.synced, v.syncedQuery, v.syncedCount = true, v.m.Query, len(vis)
	v.cursor = -1
	for i, e := range vis {
		if e.Name == v.m.Selected {
			v.cursor = i
			break
		}
	}
}

func (v *View) results(c *mui.Context) {
	m := v.m
	switch {
	case m.ListStatus == app.Failed:
		retryMessage(c, fmt.Sprintf("Could not load the Pokémon list: %v", m.ListErr), m.Retry)
	case m.ListStatus != app.Ready:
		loading(c, "Loading Pokémon…")
	case m.NoResults():
		mui.Text(c, "No Pokémon found").TextColor(c.Theme().TextMuted).Padding(12)
	default:
		v.rows(c)
	}
}

func (v *View) rows(c *mui.Context) {
	vis := v.m.Visible()
	th := c.Theme()
	v.list.Key = func(i int) any { return vis[i].ID }
	v.list.Label = func(i int) string { return vis[i].Name }
	v.list.Selected = &v.cursor

	choose := func(i int) {
		v.cursor = i
		v.m.Select(vis[i].Name)
	}
	clicked := -1
	list := mui.List(c, &v.list, len(vis)).Grow(1).Rows(func(r mui.ListRow) {
		e := vis[r.Index]
		row := mui.Row(r.Context).Gap(8).Padding(6, 10).AlignItems(mui.Center).Children(func() {
			v.thumb(r.Context, e.ID)
			mui.Textf(r.Context, "#%03d", e.ID).TextColor(rowNumberColor(th, r.Selected())).
				Width(numberColumnWidth).NoWrap()
			mui.Text(r.Context, e.Name)
		})
		if row.Clicked() {
			clicked = r.Index
		}
	})
	// A click selects; Up and Down only move the cursor, and Enter (or a
	// double click) selects the row it is on.
	switch {
	case clicked >= 0:
		choose(clicked)
	case list.Submitted() && v.cursor >= 0 && v.cursor < len(vis):
		choose(v.cursor)
	}
}

// thumb is the small sprite of a result row, or an empty slot of the same size
// until it is fetched (or if there is none). Asking for it is idempotent.
func (v *View) thumb(c *mui.Context, id int) {
	v.m.RequestThumb(id)
	if bm := v.thumbBitmap(id); bm != nil {
		mui.Image(c, bm).Size(thumbSize, thumbSize).Fit(mui.Contain)
		return
	}
	mui.Column(c).Size(thumbSize, thumbSize)
}

// thumbBitmap decodes the sprite of a row once.
func (v *View) thumbBitmap(id int) *mui.Bitmap {
	if bm, ok := v.thumbs[id]; ok {
		return bm
	}
	png, st := v.m.Thumb(id)
	if st != app.Ready {
		return nil
	}
	if len(v.thumbs) >= maxBitmaps {
		clear(v.thumbs) // decoded again, from the model's bytes, if scrolled back to
	}
	var bm *mui.Bitmap
	if b, err := mui.DecodeBitmap(png); err == nil {
		bm = b.Pixelated()
	}
	v.thumbs[id] = bm
	return bm
}

func (v *View) detail(c *mui.Context) {
	m := v.m
	switch {
	case m.Selected == "":
		mui.Text(c, "Select a Pokémon").TextColor(c.Theme().TextMuted).Padding(16)
	case m.DetailStatus == app.Failed:
		retryMessage(c, fmt.Sprintf("Could not load %s: %v", m.Selected, m.DetailErr), m.Retry)
	case m.DetailStatus != app.Ready:
		loading(c, "Loading "+display(m.Selected)+"…")
	default:
		v.card(c, m.Detail)
	}
}

// card is the detail of one Pokémon: a header band in the color of its first
// type, then the base stats in two columns.
func (v *View) card(c *mui.Context, p *pokeapi.Pokemon) {
	th := c.Theme()
	band := neutral
	if len(p.Types) > 0 {
		band = styleOf(p.Types[0])
	}
	total := 0
	for _, s := range p.Stats {
		total += s.Base
	}

	mui.Scroll(c).Grow(1).Children(func() {
		mui.Column(c).Children(func() {
			// The band color is translucent, so it sits on the theme's own
			// background in both the light and the dark appearance.
			mui.Row(c).AlignItems(mui.Center).Gap(24).Padding(24, 28).
				Background(band.Bg.Alpha(0.18)).Children(func() {
				v.sprite(c)
				mui.Column(c).Gap(6).Children(func() {
					mui.Textf(c, "#%03d", p.ID).FontSize(12).Bold().TextColor(th.TextMuted)
					mui.Text(c, display(p.Name)).FontSize(32).Bold()
					mui.Row(c).Gap(6).Children(func() {
						for _, t := range p.Types {
							s := styleOf(t)
							mui.Badge(c, t).Background(s.Bg).TextColor(s.Fg)
						}
					})
				})
			})
			// The Pokédex text, once it has arrived. Nothing is drawn while it
			// loads or if there is none, so the stats do not wait for it.
			if v.m.DescStatus == app.Ready && v.m.Description != "" {
				mui.Text(c, v.m.Description).Padding(20, 28, 0)
			}
			mui.Column(c).Padding(24, 28).Gap(16).Children(func() {
				mui.Row(c).AlignItems(mui.Center).Children(func() {
					mui.Text(c, "Base stats").Bold().Grow(1)
					mui.Textf(c, "Total %d", total).TextColor(th.TextMuted)
				})
				mui.Grid(c).Columns(2).GapX(32).GapY(18).Children(func() {
					for _, s := range p.Stats {
						mui.Column(c).Gap(6).Children(func() {
							mui.Row(c).AlignItems(mui.Center).Children(func() {
								mui.Text(c, statLabel(s.Name)).TextColor(th.TextMuted).Grow(1)
								mui.Textf(c, "%d", s.Base).FontSize(22).Bold()
							})
							mui.Progress(c, float64(s.Base)/maxBaseStat)
						})
					}
				})
			})
		})
	})
}

// sprite shows the image, or a spinner while it loads, or a placeholder if
// there is none.
func (v *View) sprite(c *mui.Context) {
	if bm := v.spriteBitmap(); bm != nil {
		mui.Image(c, bm).Size(spriteSize, spriteSize).Fit(mui.Contain)
		return
	}
	mui.Column(c).Size(spriteSize, spriteSize).Center().Radius(16).
		Background(c.Theme().Background.Alpha(0.6)).Children(func() {
		if v.m.SpriteStatus == app.Failed || v.bitmapOf != "" {
			mui.Text(c, "No image").TextColor(c.Theme().TextMuted)
		} else {
			mui.Spinner(c).Label("Loading image")
		}
	})
}

// spriteBitmap decodes the sprite of the selected Pokémon once. An image
// that does not decode counts as missing.
func (v *View) spriteBitmap() *mui.Bitmap {
	m := v.m
	if m.SpriteStatus != app.Ready {
		v.bitmap, v.bitmapOf = nil, ""
		return nil
	}
	if v.bitmapOf != m.Selected {
		v.bitmapOf = m.Selected
		if b, err := mui.DecodeBitmap(m.Sprite); err == nil {
			v.bitmap = b.Pixelated()
		} else {
			v.bitmap = nil
		}
	}
	return v.bitmap
}

// loading shows a spinner with a message.
func loading(c *mui.Context, msg string) {
	mui.Row(c).Gap(8).AlignItems(mui.Center).Padding(16).Children(func() {
		mui.Spinner(c).Label("Loading")
		mui.Text(c, msg).TextColor(c.Theme().TextMuted)
	})
}

// retryMessage shows an error with a Retry button.
func retryMessage(c *mui.Context, msg string, retry func()) {
	mui.Column(c).Gap(10).Padding(16).Children(func() {
		mui.Text(c, msg)
		mui.Button(c, "Retry").OnClick(retry)
	})
}

// display turns an API name such as "mr-mime" into "Mr Mime".
func display(name string) string {
	words := strings.Split(name, "-")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func statLabel(name string) string {
	switch name {
	case "hp":
		return "HP"
	case "special-attack":
		return "Sp. Atk"
	case "special-defense":
		return "Sp. Def"
	}
	return display(name)
}

// rowNumberColor is the color of the Pokédex number in a result row: muted,
// except in the selected row, where it must stay readable on the accent.
func rowNumberColor(th *mui.Theme, selected bool) mui.Color {
	if selected {
		return th.AccentText
	}
	return th.TextMuted
}
