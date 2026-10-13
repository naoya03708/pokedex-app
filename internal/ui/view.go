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
	spriteSize  = 120
)

// View draws the search box, the result list and the detail pane. Its fields
// are widget state that lasts between frames: never an element or context.
type View struct {
	m *app.Model

	split  float32 // width of the result list pane
	list   mui.ListState
	cursor int // highlighted row of the list, -1 for none

	// The cursor follows the selection when the visible rows change.
	synced      bool
	syncedQuery string
	syncedCount int

	// The decoded sprite of bitmapOf, made once rather than in every frame.
	bitmap   *mui.Bitmap
	bitmapOf string
}

// New returns a View of m.
func New(m *app.Model) *View { return &View{m: m, split: 300, cursor: -1} }

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
	muted := c.Theme().TextMuted
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
		row := mui.Row(r.Context).Gap(8).Padding(6, 10).Children(func() {
			mui.Textf(r.Context, "#%03d", e.ID).TextColor(muted).Width(44)
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

func (v *View) card(c *mui.Context, p *pokeapi.Pokemon) {
	muted := c.Theme().TextMuted
	mui.Column(c).Padding(16).Gap(14).Children(func() {
		mui.Row(c).Gap(16).AlignItems(mui.Center).Children(func() {
			v.sprite(c)
			mui.Column(c).Gap(6).Children(func() {
				mui.Text(c, display(p.Name)).FontSize(24).Bold()
				mui.Textf(c, "#%03d", p.ID).TextColor(muted)
				mui.Row(c).Gap(6).Children(func() {
					for _, t := range p.Types {
						mui.Badge(c, t)
					}
				})
			})
		})
		mui.Text(c, "Base stats").Bold()
		mui.Column(c).Gap(6).Children(func() {
			for _, s := range p.Stats {
				mui.Row(c).Gap(10).AlignItems(mui.Center).Children(func() {
					mui.Text(c, statLabel(s.Name)).Width(64)
					mui.Textf(c, "%d", s.Base).Width(36).TextAlign(mui.End)
					mui.Progress(c, float64(s.Base)/maxBaseStat).Grow(1)
				})
			}
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
	mui.Column(c).Size(spriteSize, spriteSize).Center().Children(func() {
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
