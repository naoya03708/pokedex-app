package ui_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	mui "github.com/egoist/mygo/ui"

	"github.com/naoya03708/pokedex-app/internal/pokeapi"
)

// advSets are the ID sets the fake API answers with.
var advSets = map[string][]int{
	"type/electric": {25, 26},
	"type/fire":     {},
	"generation/1":  {1, 25},
}

func advMembers(_ context.Context, kind pokeapi.Kind, key string) ([]int, error) {
	if ids, ok := advSets[string(kind)+"/"+key]; ok {
		return ids, nil
	}
	return nil, pokeapi.ErrNotFound
}

// clickAndSettle clicks s and lets the animation it starts (the panel opening
// or closing) finish: the Tester's clock stands still until it is advanced.
func (h *harness) clickAndSettle(s string) {
	h.t.Helper()
	h.click(s)
	h.tt.Advance(time.Second)
}

// chooseOption opens the drop-down showing current and picks option.
func (h *harness) chooseOption(current, option string) {
	h.t.Helper()
	h.clickAndSettle(current)
	h.clickAndSettle(option)
}

// openAdvanced starts the app and opens the advanced search panel.
func openAdvanced(t *testing.T, src fakeSource) *harness {
	t.Helper()
	if src.members == nil {
		src.members = advMembers
	}
	h := newHarness(t, src)
	h.start()
	h.clickAndSettle("Advanced search")
	return h
}

func TestUI_FR7_PanelIsClosedByDefaultAndTheListIsUnfiltered(t *testing.T) {
	h := newHarness(t, fakeSource{})
	h.start()

	h.wantText("Search Pokémon")
	h.wantText("Advanced search")
	h.wantNoText("Any type")
	h.wantNoText("Clear filters")
	for _, n := range []string{"bulbasaur", "pikachu", "raichu", "mr-mime"} {
		h.wantText(n)
	}
}

func TestUI_FR7_OpeningThePanelShowsTheThreeConditionsSetToAny(t *testing.T) {
	h := openAdvanced(t, fakeSource{})

	for _, s := range []string{"Any generation", "Any type", "Any egg group", "Clear filters"} {
		h.wantText(s)
	}
	h.wantText("bulbasaur") // no condition: the list is unchanged
}

func TestUI_FR7_ChoosingATypeFiltersTheList(t *testing.T) {
	h := openAdvanced(t, fakeSource{})

	h.chooseOption("Any type", "Electric")
	h.tt.Frame()
	h.wantText("Loading filter…") // while the set is fetched
	h.wantNoText("pikachu")
	h.run(1)

	h.wantNoText("Loading filter…")
	h.wantText("pikachu")
	h.wantText("raichu")
	h.wantNoText("bulbasaur")
	h.wantNoText("mr-mime")
}

func TestUI_FR7_ConditionsAndTheNameSearchAreCombined(t *testing.T) {
	h := openAdvanced(t, fakeSource{})
	h.chooseOption("Any type", "Electric")
	h.run(1)
	h.chooseOption("Any generation", "Gen 1 · Kanto")
	h.run(1)

	h.wantText("pikachu")
	h.wantNoText("raichu") // electric, but not Gen 1

	h.click("Search Pokémon")
	h.tt.Type("zzz")
	h.wantText("No Pokémon found")
}

func TestUI_FR3_FR7_NoMatchShowsNoResults(t *testing.T) {
	h := openAdvanced(t, fakeSource{})

	h.chooseOption("Any type", "Fire")
	h.run(1)

	h.wantText("No Pokémon found")
}

func TestUI_FR7_ClearRestoresTheListAndKeepsTheNameSearch(t *testing.T) {
	h := openAdvanced(t, fakeSource{})
	h.click("Search Pokémon")
	h.tt.Type("a") // bulbasaur, pikachu, raichu
	h.chooseOption("Any type", "Electric")
	h.run(1)
	h.wantNoText("bulbasaur")

	h.click("Clear filters")

	h.wantText("Any type")
	h.wantText("bulbasaur")
	h.wantText("pikachu")
	h.wantNoText("mr-mime") // the name search "a" is still in force
}

func TestUI_FR7_ClosingThePanelTurnsTheConditionsOff(t *testing.T) {
	h := openAdvanced(t, fakeSource{})
	h.chooseOption("Any type", "Electric")
	h.run(1)
	h.wantNoText("bulbasaur")

	h.clickAndSettle("Advanced search")

	h.wantNoText("Clear filters")
	h.wantText("bulbasaur")

	// Opening it again brings the choice back
	h.clickAndSettle("Advanced search")
	h.wantText("Electric")
	h.wantNoText("bulbasaur")
}

func TestUI_FR3_FR7_FailureShowsMessageAndRetry(t *testing.T) {
	var calls atomic.Int32
	h := openAdvanced(t, fakeSource{members: func(ctx context.Context, k pokeapi.Kind, key string) ([]int, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("network is down")
		}
		return advMembers(ctx, k, key)
	}})

	h.chooseOption("Any type", "Electric")
	h.run(1)

	h.wantText("Retry")
	h.wantNoText("No Pokémon found")
	h.wantNoText("pikachu")

	h.click("Retry")
	h.wantText("Loading filter…")
	h.run(1)

	h.wantNoText("Retry")
	h.wantText("pikachu")
	h.wantNoText("bulbasaur")
}

func TestUI_FR7_PanelAndDropDownWorkFromTheKeyboard(t *testing.T) {
	// Given the loaded app with the panel closed
	h := newHarness(t, fakeSource{members: advMembers})
	h.start()

	// When Tab reaches the toggle and Enter opens it
	h.tt.Key(0, mui.KeyTab) // search box
	h.tt.Key(0, mui.KeyTab) // toggle
	h.tt.Key(0, mui.KeyEnter)
	h.tt.Advance(time.Second)
	h.wantText("Any type")

	// And Tab reaches the type drop-down, which opens with Down and chooses with Enter
	h.tt.Key(0, mui.KeyTab)  // generation
	h.tt.Key(0, mui.KeyTab)  // type
	h.tt.Key(0, mui.KeyDown) // opens the popup on "Any type"
	for range 4 {
		h.tt.Key(0, mui.KeyDown) // Normal, Fire, Water, Electric
	}
	h.tt.Key(0, mui.KeyEnter)
	h.tt.Advance(time.Second)

	// Then the list is filtered
	h.run(1)
	h.wantText("pikachu")
	h.wantNoText("bulbasaur")
}

func TestUI_FR7_CachedConditionRespondsWithin100ms(t *testing.T) {
	// Given 1,300 Pokémon and two sets that were already fetched
	many := make([]pokeapi.Entry, 1300)
	for i := range many {
		many[i] = pokeapi.Entry{ID: i + 1, Name: fmt.Sprintf("mon%04d", i+1)}
	}
	h := openAdvanced(t, fakeSource{
		list: func(context.Context) ([]pokeapi.Entry, error) { return many, nil },
		members: func(_ context.Context, _ pokeapi.Kind, key string) ([]int, error) {
			if key == "electric" {
				return []int{1, 2, 3}, nil
			}
			return []int{4, 5, 6}, nil
		},
	})
	h.chooseOption("Any type", "Electric")
	h.run(1)
	h.chooseOption("Electric", "Fire")
	h.run(1)
	h.click("Fire") // open the drop-down again

	// When the cached condition is chosen again
	start := time.Now()
	h.click("Electric")
	elapsed := time.Since(start)

	// Then the list has changed within the limit
	if limit := 100 * time.Millisecond * slowdown; elapsed > limit {
		t.Errorf("choosing a cached condition took %v, want <= %v", elapsed, limit)
	}
	h.wantText("mon0001")
	h.wantNoText("mon0004")
}
