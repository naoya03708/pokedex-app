package ui_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"slices"
	"sync"
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
	// description is optional: Pokémon without a SpeciesID never ask for one.
	description func(ctx context.Context, p *pokeapi.Pokemon) (string, error)
	// spriteByID is for the result rows. The harness default never answers, so
	// the rows' fetches do not interleave with the posts other tests count.
	spriteByID func(ctx context.Context, id int) ([]byte, error)
}

func (f fakeSource) List(ctx context.Context) ([]pokeapi.Entry, error) { return f.list(ctx) }
func (f fakeSource) Get(ctx context.Context, n string) (*pokeapi.Pokemon, error) {
	return f.get(ctx, n)
}
func (f fakeSource) Sprite(ctx context.Context, p *pokeapi.Pokemon) ([]byte, error) {
	return f.sprite(ctx, p)
}

func (f fakeSource) Description(ctx context.Context, p *pokeapi.Pokemon) (string, error) {
	return f.description(ctx, p)
}

func (f fakeSource) SpriteByID(ctx context.Context, id int) ([]byte, error) {
	return f.spriteByID(ctx, id)
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
	if src.spriteByID == nil {
		src.spriteByID = func(ctx context.Context, _ int) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
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

func TestUI_FR2_ShowsTheTotalOfTheBaseStats(t *testing.T) {
	h := newHarness(t, fakeSource{})
	h.start()

	h.click("pikachu")
	h.run(1)

	h.wantText("Total 321") // 35 + 55 + 40 + 50 + 51 + 90
}

func TestUI_FR2_ShowsBothTypesOfADualTypePokemon(t *testing.T) {
	h := newHarness(t, fakeSource{get: func(_ context.Context, n string) (*pokeapi.Pokemon, error) {
		p := *pikachu
		p.Name, p.Types = n, []string{"fire", "flying"}
		return &p, nil
	}})
	h.start()

	h.click("pikachu")
	h.run(1)

	h.wantText("fire")
	h.wantText("flying")
}

func TestUI_FR2_DetailRendersInDarkModeAndAtTheMinimumWindowSize(t *testing.T) {
	h := newHarness(t, fakeSource{})
	h.start()
	h.click("pikachu")
	h.run(2)

	h.tt.SetDark(true)
	h.tt.SetSize(640, 420)
	h.tt.Frame()

	for _, s := range []string{"Pikachu", "Total 321", "Speed", "90"} {
		h.wantText(s)
	}
}

func TestUI_FR2_LongNameAndTwoStatColumnsFitTheMinimumWindow(t *testing.T) {
	// Given a long name and two types in the smallest window (see main.go)
	const winW, winH = 640, 420
	h := newHarness(t, fakeSource{get: func(_ context.Context, n string) (*pokeapi.Pokemon, error) {
		p := *pikachu
		p.Name, p.Types = "landorus-therian", []string{"ground", "flying"}
		return &p, nil
	}})
	h.start()
	h.click("pikachu")
	h.run(2)

	// When the window has its minimum size
	h.tt.SetSize(winW, winH)
	h.tt.Frame()

	// Then the name stays inside the window, and the stats form two even columns
	name, ok := h.tt.Find("Landorus Therian")
	if !ok || name.X+name.W > winW {
		t.Errorf("name box %+v does not fit the %d wide window", name, winW)
	}
	hp, _ := h.tt.Find("HP")
	attack, _ := h.tt.Find("Attack")
	speed, _ := h.tt.Find("Speed")
	if attack.X <= hp.X || speed.X != attack.X {
		t.Errorf("stats are not in two columns: HP x=%.0f, Attack x=%.0f, Speed x=%.0f", hp.X, attack.X, speed.X)
	}
	if speed.Y+speed.H > winH {
		t.Errorf("last stat row ends at %.0f, below the %d high window", speed.Y+speed.H, winH)
	}
}

func withSpeciesDetail(ctx context.Context, name string) (*pokeapi.Pokemon, error) {
	p, _ := detailFor(ctx, name)
	p.SpeciesID = 25
	return p, nil
}

func TestUI_FR5_DescriptionAppearsAfterTheDetail(t *testing.T) {
	h := newHarness(t, fakeSource{get: withSpeciesDetail,
		description: func(context.Context, *pokeapi.Pokemon) (string, error) { return "It stores electricity.", nil }})
	h.start()

	h.click("pikachu")
	h.run(1)
	h.wantText("Base stats")
	h.wantNoText("It stores electricity.")
	h.run(2) // sprite and description, in either order

	h.wantText("It stores electricity.")
}

func TestUI_FR5_FailedDescriptionLeavesNoGapOrError(t *testing.T) {
	h := newHarness(t, fakeSource{get: withSpeciesDetail,
		description: func(context.Context, *pokeapi.Pokemon) (string, error) { return "", errors.New("offline") }})
	h.start()

	h.click("pikachu")
	h.run(3)

	h.wantText("Base stats")
	h.wantNoText("offline")
	h.wantNoText("Retry")
}

// magenta is a color the theme never uses, so finding it means a thumbnail was drawn.
var magenta = color.RGBA{R: 255, B: 255, A: 255}

func solidPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// thumbsDrawn counts magenta pixels in the result list (the left 260 DIPs).
func (h *harness) thumbsDrawn() int {
	img := h.tt.Image()
	n := 0
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < min(img.Rect.Max.X, img.Rect.Min.X+260); x++ {
			if img.RGBAAt(x, y) == magenta {
				n++
			}
		}
	}
	return n
}

func TestUI_FR6_RowsShowTheirSpriteOnceFetched(t *testing.T) {
	img := solidPNG(t, magenta)
	h := newHarness(t, fakeSource{spriteByID: func(context.Context, int) ([]byte, error) { return img, nil }})
	h.start()
	if got := h.thumbsDrawn(); got != 0 {
		t.Fatalf("drawn before any fetch: %d", got)
	}

	h.run(len(entries)) // one result per visible row

	if h.thumbsDrawn() == 0 {
		t.Fatal("no thumbnail was drawn")
	}
	h.wantText("pikachu")
	h.wantText("#025")
}

func TestUI_FR6_RowsWithoutSpriteKeepTheirLayout(t *testing.T) {
	// Given the same list, once with sprites and once where every fetch fails
	img := solidPNG(t, magenta)
	with := newHarness(t, fakeSource{spriteByID: func(context.Context, int) ([]byte, error) { return img, nil }})
	with.start()
	with.run(len(entries))
	without := newHarness(t, fakeSource{spriteByID: func(context.Context, int) ([]byte, error) {
		return nil, pokeapi.ErrNotFound
	}})
	without.start()
	without.run(len(entries))

	// Then every row has the same position for its number and name
	for _, s := range []string{"#001", "#025", "#122", "mr-mime"} {
		a, ok1 := with.tt.Find(s)
		b, ok2 := without.tt.Find(s)
		if !ok1 || !ok2 || a != b {
			t.Errorf("%s moved: %v (%v) vs %v (%v)", s, a, ok1, b, ok2)
		}
	}
	if without.thumbsDrawn() != 0 {
		t.Error("a failed fetch drew something")
	}
}

func TestUI_FR6_OnlyBuiltRowsAreFetched(t *testing.T) {
	// Given a list far longer than the window
	var fetched sync.Map
	big := make([]pokeapi.Entry, 500)
	for i := range big {
		big[i] = pokeapi.Entry{ID: i + 1, Name: fmt.Sprintf("mon-%d", i+1)}
	}
	h := newHarness(t, fakeSource{
		list: func(context.Context) ([]pokeapi.Entry, error) { return big, nil },
		spriteByID: func(ctx context.Context, id int) ([]byte, error) {
			fetched.Store(id, true)
			<-ctx.Done()
			return nil, ctx.Err()
		}})

	h.start()
	time.Sleep(100 * time.Millisecond)

	// Then far rows were never requested
	n := 0
	fetched.Range(func(k, _ any) bool { n++; return true })
	if n == 0 || n > 40 {
		t.Fatalf("fetched %d sprites for a window showing about 15 rows", n)
	}
	if _, far := fetched.Load(400); far {
		t.Fatal("a row far below the window was fetched")
	}
}

func TestUI_FR6_ClickingARowWithASpriteStillShowsItsDetail(t *testing.T) {
	img := solidPNG(t, magenta)
	h := newHarness(t, fakeSource{spriteByID: func(context.Context, int) ([]byte, error) { return img, nil }})
	h.start()
	h.run(len(entries))

	h.click("pikachu")

	h.wantText("#025")
	h.wantText("Pikachu")
}

func TestUI_FR6_UndecodableImageLeavesTheRowAsIfThereWasNone(t *testing.T) {
	h := newHarness(t, fakeSource{spriteByID: func(context.Context, int) ([]byte, error) {
		return []byte("\x89PNG\r\n\x1a\ntruncated"), nil
	}})
	h.start()

	h.run(len(entries))

	h.wantText("pikachu")
	if h.thumbsDrawn() != 0 {
		t.Error("something was drawn for an image that does not decode")
	}
}

// thumbColumns returns the first and last x of the magenta pixels in the
// horizontal band of a result row.
func (h *harness) thumbColumns(r mui.Rect) (first, last int) {
	img := h.tt.Image()
	first, last = -1, -1
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := img.Rect.Min.X; x < min(img.Rect.Max.X, img.Rect.Min.X+260); x++ {
			if img.RGBAAt(x, y) == magenta {
				if first < 0 {
					first = x
				}
				last = x
			}
		}
	}
	return first, last
}

func TestUI_FR6_RowOrderIsNumberNameThenSprite(t *testing.T) {
	img := solidPNG(t, magenta)
	h := newHarness(t, fakeSource{spriteByID: func(context.Context, int) ([]byte, error) { return img, nil }})
	h.start()
	h.run(len(entries))

	num, ok1 := h.tt.Find("#025")
	row, ok2 := h.tt.Find("pikachu") // the clickable row, not the name text
	if !ok1 || !ok2 {
		t.Fatalf("missing: %v %v", ok1, ok2)
	}
	first, last := h.thumbColumns(mui.Rect{Y: row.Y, H: row.H})

	// Then the sprite is right of the number, and ends at the row's right
	// padding, the same place in every row
	if first < 0 || float32(first) < num.X+num.W {
		t.Errorf("sprite starts at x=%d, left of the end of the number %v", first, num.X+num.W)
	}
	if want := row.X + row.W - 10; float32(last) < want-6 || float32(last) > want {
		t.Errorf("sprite ends at x=%d, want about %v (the row's right edge)", last, want)
	}
	other, _ := h.tt.Find("mr-mime")
	_, lastOther := h.thumbColumns(mui.Rect{Y: other.Y, H: other.H})
	if last != lastOther {
		t.Errorf("sprites are not aligned at the right: %d vs %d", last, lastOther)
	}
}

func TestUI_FR6_LongNameKeepsTheSpriteInsideTheListAndTheRowHeight(t *testing.T) {
	img := solidPNG(t, magenta)
	long := []pokeapi.Entry{{ID: 25, Name: "pikachu"}, {ID: 10196, Name: "charizard-mega-x-gigantamax"}}
	h := newHarness(t, fakeSource{
		list:       func(context.Context) ([]pokeapi.Entry, error) { return long, nil },
		spriteByID: func(context.Context, int) ([]byte, error) { return img, nil }})
	h.start()
	h.run(len(long))

	short, ok1 := h.tt.Find("pikachu")
	name, ok2 := h.tt.Find("charizard-mega-x-gigantamax")
	if !ok1 || !ok2 {
		t.Fatalf("rows not found: %v %v", ok1, ok2)
	}
	first, last := h.thumbColumns(mui.Rect{Y: name.Y, H: name.H})

	if first < 0 {
		t.Fatal("the sprite was pushed out of the list (nothing drawn in the row)")
	}
	if last >= 260 {
		t.Errorf("sprite right edge x=%d is outside the 260 wide list", last)
	}
	if name.H != short.H {
		t.Errorf("a long name changed the row height: %v vs %v (it must end with an ellipsis, not wrap)", name.H, short.H)
	}
}
