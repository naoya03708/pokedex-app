package app_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/naoya03708/pokedex-app/internal/app"
	"github.com/naoya03708/pokedex-app/internal/pokeapi"
)

// nameList is in number order, like the list the app loads. It holds names
// with symbols in them and names that only contain "pi" or "mim".
var nameList = []pokeapi.Entry{
	{ID: 25, Name: "pikachu"}, {ID: 29, Name: "nidoran-f"}, {ID: 83, Name: "farfetchd"},
	{ID: 122, Name: "mr-mime"}, {ID: 124, Name: "jynx"}, {ID: 167, Name: "spinarak"},
	{ID: 172, Name: "pichu"}, {ID: 250, Name: "ho-oh"}, {ID: 439, Name: "mime-jr"},
	{ID: 772, Name: "type-null"}, {ID: 778, Name: "mimikyu"},
}

func TestFilter_FR8_SpellingVariantsMatchHyphenatedNames(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"mr mime", "mr-mime"},
		{"Mr.Mime", "mr-mime"},
		{"mr-mime", "mr-mime"},
		{"MRMIME", "mr-mime"},
		{"farfetch'd", "farfetchd"},
		{"type: null", "type-null"},
		{"ho oh", "ho-oh"},
		{"nidoran f", "nidoran-f"},
	} {
		// Given a query spelled with spaces or symbols
		// When the list is filtered
		got := names(app.Filter(nameList, tc.query))

		// Then the hyphenated name is found
		if !slices.Contains(got, tc.want) {
			t.Errorf("query %q: got %v, want %q among them", tc.query, got, tc.want)
		}
	}
}

func TestFilter_FR8_PrefixMatchesComeFirst(t *testing.T) {
	// Given "pi", which starts pikachu and pichu and is inside spinarak
	// When the list is filtered
	got := names(app.Filter(nameList, "pi"))

	// Then the prefix group comes first (in number order), then the rest
	want := []string{"pikachu", "pichu", "spinarak"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilter_FR8_MimRanksPrefixThenInside(t *testing.T) {
	got := names(app.Filter(nameList, "mim"))

	want := []string{"mime-jr", "mimikyu", "mr-mime"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilter_FR8_PrefixIgnoresSymbolsInTheName(t *testing.T) {
	// A name beginning "mr-m..." starts with "mrm" once symbols are dropped
	got := names(app.Filter(nameList, "mrm"))

	if !slices.Equal(got, []string{"mr-mime"}) {
		t.Fatalf("got %v", got)
	}
}

func TestFilter_FR8_QueryWithoutLettersOrDigitsIsEmpty(t *testing.T) {
	for _, q := range []string{"-", " - ", "...", "'"} {
		got := app.Filter(nameList, q)

		if len(got) != len(nameList) {
			t.Fatalf("query %q: got %d entries, want all %d", q, len(got), len(nameList))
		}
		for i := range nameList {
			if got[i] != nameList[i] {
				t.Fatalf("query %q: order changed at %d", q, i)
			}
		}
	}
}

func TestFilter_FR8_NoMatchStaysEmpty(t *testing.T) {
	if got := app.Filter(nameList, "pika chu x"); len(got) != 0 {
		t.Fatalf("got %v", names(got))
	}
}

func TestFilter_FR8_DoesNotChangeTheInputList(t *testing.T) {
	in := slices.Clone(nameList)

	app.Filter(in, "pi")

	if !slices.Equal(in, nameList) {
		t.Fatal("Filter reordered its input")
	}
}

func TestFilter_FR8_FullListStaysUnder100ms(t *testing.T) {
	// Given about 1,300 names
	big := make([]pokeapi.Entry, 0, 1300)
	for i := 1; i <= 1300; i++ {
		big = append(big, pokeapi.Entry{ID: i, Name: "mon-" + string(rune('a'+i%26)) + "-x"})
	}

	// When a query is filtered
	start := time.Now()
	app.Filter(big, "mon a")

	// Then it is fast enough to run on each key press
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("took %v", d)
	}
}

func TestModel_FR8_AdvancedConditionsKeepTheRanking(t *testing.T) {
	// Given "pi" ranks pichu (172) before spinarak (167) although spinarak has
	// the lower number, and a type set that holds all three
	list := []pokeapi.Entry{{ID: 25, Name: "pikachu"}, {ID: 167, Name: "spinarak"}, {ID: 172, Name: "pichu"}, {ID: 300, Name: "skitty"}}
	m, q := newModel(fakeSource{
		list: func(context.Context) ([]pokeapi.Entry, error) { return list, nil },
		members: func(context.Context, pokeapi.Kind, string) ([]int, error) {
			return []int{25, 167, 172}, nil
		},
	})
	t.Cleanup(m.Close)
	m.Start()
	q.run(t)
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "electric")
	m.SyncAdvanced()
	q.run(t)

	// When the query is typed
	m.Query = "pi"

	// Then the set only removes rows (skitty is not in it); the ranking stays
	got := names(m.Visible())
	if want := []string{"pikachu", "pichu", "spinarak"}; !slices.Equal(got, want) {
		t.Fatalf("visible = %v, want %v", got, want)
	}
}
