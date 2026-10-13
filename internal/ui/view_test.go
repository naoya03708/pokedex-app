package ui_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	mui "github.com/egoist/mygo/ui"

	"github.com/naoya03708/pokedex-app/internal/app"
	"github.com/naoya03708/pokedex-app/internal/pokeapi"
	"github.com/naoya03708/pokedex-app/internal/ui"
)

var entries = []pokeapi.Entry{
	{ID: 1, Name: "bulbasaur"}, {ID: 25, Name: "pikachu"}, {ID: 26, Name: "raichu"}, {ID: 122, Name: "mr-mime"},
}

var pikachu = &pokeapi.Pokemon{
	ID: 25, Name: "pikachu", Types: []string{"electric"}, SpriteURL: "https://img.example/25.png",
	Stats: []pokeapi.Stat{
		{Name: "hp", Base: 35}, {Name: "attack", Base: 55}, {Name: "defense", Base: 40},
		{Name: "special-attack", Base: 50}, {Name: "special-defense", Base: 51}, {Name: "speed", Base: 90},
	},
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeSource lets each test decide what the API does.
type fakeSource struct {
	list   func(ctx context.Context) ([]pokeapi.Entry, error)
	get    func(ctx context.Context, name string) (*pokeapi.Pokemon, error)
	sprite func(ctx context.Context, p *pokeapi.Pokemon) ([]byte, error)
}

func (f fakeSource) List(ctx context.Context) ([]pokeapi.Entry, error) { return f.list(ctx) }
func (f fakeSource) Get(ctx context.Context, n string) (*pokeapi.Pokemon, error) {
	return f.get(ctx, n)
}
func (f fakeSource) Sprite(ctx context.Context, p *pokeapi.Pokemon) ([]byte, error) {
	return f.sprite(ctx, p)
}

func okList(context.Context) ([]pokeapi.Entry, error) { return entries, nil }

// detailFor returns a pikachu-shaped detail under the requested name.
func detailFor(_ context.Context, name string) (*pokeapi.Pokemon, error) {
	p := *pikachu
	p.Name = name
	return &p, nil
}

// harness drives the view like a user. Posted functions run only when the
// test, acting as the UI thread, asks for them.
type harness struct {
	t  *testing.T
	m  *app.Model
	tt *mui.Tester
	q  chan func()
}

func newHarness(t *testing.T, src fakeSource) *harness {
	t.Helper()
	if src.list == nil {
		src.list = okList
	}
	if src.get == nil {
		src.get = detailFor
	}
	if src.sprite == nil {
		img := pngBytes(t)
		src.sprite = func(context.Context, *pokeapi.Pokemon) ([]byte, error) { return img, nil }
	}
	h := &harness{t: t, q: make(chan func(), 32)}
	h.m = app.New(src, func(fn func()) { h.q <- fn })
	h.tt = mui.NewTester(ui.New(h.m).Build, 900, 600)
	t.Cleanup(h.m.Close)
	return h
}

// run applies n posted results on the UI thread and draws a frame.
func (h *harness) run(n int) {
	h.t.Helper()
	for range n {
		select {
		case fn := <-h.q:
			fn()
		case <-time.After(2 * time.Second):
			h.t.Fatal("nothing was posted to the UI thread")
		}
	}
	h.tt.Frame()
}

// start loads the list, as the window does when it opens.
func (h *harness) start() {
	h.m.Start()
	h.run(1)
}

func (h *harness) click(s string) {
	h.t.Helper()
	if err := h.tt.Click(s); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) wantText(s string) {
	h.t.Helper()
	if !h.tt.HasText(s) {
		h.t.Errorf("want text %q, have %q", s, h.tt.Texts())
	}
}

func (h *harness) wantNoText(s string) {
	h.t.Helper()
	if h.tt.HasText(s) {
		h.t.Errorf("unexpected text %q in %q", s, h.tt.Texts())
	}
}

func TestUI_FR3_ShowsLoadingUntilListArrives(t *testing.T) {
	h := newHarness(t, fakeSource{})

	h.m.Start()
	h.tt.Frame()
	h.wantText("Loading Pokémon…")
	h.wantNoText("pikachu")

	h.run(1)
	h.wantNoText("Loading Pokémon…")
	h.wantText("pikachu")
}

func TestUI_FR1_EmptyQueryListsAllInNumberOrder(t *testing.T) {
	h := newHarness(t, fakeSource{})

	h.start()

	texts := h.tt.Texts()
	var got []string
	for _, n := range []string{"bulbasaur", "pikachu", "raichu", "mr-mime"} {
		i := slices.Index(texts, n)
		if i < 0 {
			t.Fatalf("%q missing in %q", n, texts)
		}
		got = append(got, fmt.Sprintf("%03d:%s", i, n))
	}
	if !slices.IsSorted(got) {
		t.Errorf("names are not in number order: %v", got)
	}
	h.wantText("#025")
}

func TestUI_FR1_TypingFiltersCaseInsensitively(t *testing.T) {
	h := newHarness(t, fakeSource{})
	h.start()

	h.click("Search Pokémon")
	h.tt.Type("PIKA")

	h.wantText("pikachu")
	h.wantNoText("bulbasaur")
	h.wantNoText("raichu")
}

func TestUI_FR1_FilterRespondsWithin100ms(t *testing.T) {
	many := make([]pokeapi.Entry, 1300)
	for i := range many {
		many[i] = pokeapi.Entry{ID: i + 1, Name: fmt.Sprintf("mon%04d", i+1)}
	}
	h := newHarness(t, fakeSource{list: func(context.Context) ([]pokeapi.Entry, error) { return many, nil }})
	h.start()
	h.click("Search Pokémon")

	start := time.Now()
	h.tt.Type("1")
	elapsed := time.Since(start)

	if limit := 100 * time.Millisecond * slowdown; elapsed > limit {
		t.Errorf("typing one character took %v, want <= %v", elapsed, limit)
	}
	h.wantText("mon0001")
}

func TestUI_FR3_NoResultsMessage(t *testing.T) {
	h := newHarness(t, fakeSource{})
	h.start()

	h.click("Search Pokémon")
	h.tt.Type("zzz")

	h.wantText("No Pokémon found")
	h.wantNoText("pikachu")
}

func TestUI_FR3_ListErrorShowsMessageAndRetry(t *testing.T) {
	var calls atomic.Int32
	h := newHarness(t, fakeSource{list: func(context.Context) ([]pokeapi.Entry, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("network is down")
		}
		return entries, nil
	}})
	h.m.Start()
	h.run(1)
	h.wantText("Could not load the Pokémon list: network is down")
	h.wantNoText("Loading Pokémon…")

	h.click("Retry")
	h.tt.Frame()
	h.wantText("Loading Pokémon…")
	h.run(1)

	h.wantText("pikachu")
	h.wantNoText("Retry")
}

func TestUI_FR2_ShowsHintBeforeAnySelection(t *testing.T) {
	h := newHarness(t, fakeSource{})

	h.start()

	h.wantText("Select a Pokémon")
}

func TestUI_FR2_ClickingARowShowsDetail(t *testing.T) {
	h := newHarness(t, fakeSource{})
	h.start()

	h.click("pikachu")
	h.tt.Frame()
	h.wantText("Loading Pikachu…")
	h.run(1)

	for _, s := range []string{"Pikachu", "#025", "electric", "HP", "35", "Attack", "55", "Defense", "40",
		"Sp. Atk", "50", "Sp. Def", "51", "Speed", "90"} {
		h.wantText(s)
	}
	h.wantNoText("Loading Pikachu…")
	h.wantNoText("Select a Pokémon")
}

func TestUI_FR2_SpriteFailureShowsPlaceholderKeepingDetail(t *testing.T) {
	h := newHarness(t, fakeSource{sprite: func(context.Context, *pokeapi.Pokemon) ([]byte, error) {
		return nil, errors.New("no image")
	}})
	h.start()

	h.click("pikachu")
	h.run(1) // detail text
	h.wantText("Pikachu")
	h.run(1) // sprite result: failed

	h.wantText("Pikachu")
	h.wantText("No image")
}

func TestUI_FR2_SpriteReplacesPlaceholder(t *testing.T) {
	h := newHarness(t, fakeSource{})
	h.start()

	h.click("pikachu")
	h.run(2)

	h.wantText("Pikachu")
	h.wantNoText("No image")
}

func TestUI_FR2_KeyboardArrowsMoveAndEnterSelects(t *testing.T) {
	var gets atomic.Int32
	h := newHarness(t, fakeSource{get: func(ctx context.Context, n string) (*pokeapi.Pokemon, error) {
		gets.Add(1)
		return detailFor(ctx, n)
	}})
	h.start()

	h.tt.Key(0, mui.KeyTab) // search box
	h.tt.Key(0, mui.KeyTab) // result list
	h.tt.Key(0, mui.KeyDown)
	h.tt.Key(0, mui.KeyDown) // second row: pikachu
	h.tt.Frame()
	if gets.Load() != 0 || h.m.Selected != "" {
		t.Fatalf("moving with the arrows must not select: gets=%d selected=%q", gets.Load(), h.m.Selected)
	}
	h.tt.Key(0, mui.KeyEnter)
	h.run(1)

	if h.m.Selected != "pikachu" {
		t.Fatalf("selected = %q, want pikachu", h.m.Selected)
	}
	h.wantText("Pikachu")
}

func TestUI_FR3_DetailErrorShowsMessageAndRetry(t *testing.T) {
	var calls atomic.Int32
	h := newHarness(t, fakeSource{get: func(ctx context.Context, n string) (*pokeapi.Pokemon, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("timeout")
		}
		return detailFor(ctx, n)
	}})
	h.start()

	h.click("pikachu")
	h.run(1)
	h.wantText("Could not load pikachu: timeout")

	h.click("Retry")
	h.run(2)

	h.wantText("Pikachu")
	h.wantNoText("Retry")
}

func TestUI_FR4_StaleDetailIsNotShown(t *testing.T) {
	release := map[string]chan struct{}{"pikachu": make(chan struct{}), "raichu": make(chan struct{})}
	h := newHarness(t, fakeSource{get: func(ctx context.Context, n string) (*pokeapi.Pokemon, error) {
		<-release[n]
		return detailFor(ctx, n)
	}})
	h.start()

	h.click("pikachu")
	h.click("raichu")
	close(release["pikachu"])
	h.run(1) // the stale result
	h.wantNoText("Pikachu")
	h.wantText("Loading Raichu…")

	close(release["raichu"])
	h.run(2)
	h.wantText("Raichu")
	h.wantNoText("Pikachu")
}

func TestUI_FR2_ArrowsContinueFromTheClickedRow(t *testing.T) {
	h := newHarness(t, fakeSource{})
	h.start()
	h.click("pikachu")
	h.run(2)

	h.tt.Key(0, mui.KeyDown) // raichu, the row after pikachu
	h.tt.Key(0, mui.KeyEnter)
	h.run(1)

	if h.m.Selected != "raichu" {
		t.Fatalf("selected = %q, want raichu", h.m.Selected)
	}
}
