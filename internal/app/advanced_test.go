package app_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/naoya03708/pokedex-app/internal/app"
	"github.com/naoya03708/pokedex-app/internal/pokeapi"
)

// withForm is the list plus a form (#10080), which has no species of its own.
var withForm = append(slices.Clone(entries), pokeapi.Entry{ID: 10080, Name: "pikachu-cosplay"})

// sets answers the advanced search with fixed ID sets.
var sets = map[string][]int{
	"type/electric":   {25, 26, 10080},
	"generation/1":    {1, 25},
	"egg-group/fairy": {25, 122},
}

func okMembers(_ context.Context, kind pokeapi.Kind, key string) ([]int, error) {
	if ids, ok := sets[string(kind)+"/"+key]; ok {
		return ids, nil
	}
	return nil, pokeapi.ErrNotFound
}

func label(t *testing.T, cs []app.Choice, key string) string {
	t.Helper()
	for _, c := range cs {
		if c.Key == key {
			return c.Label
		}
	}
	t.Fatalf("no choice with key %q in %v", key, cs)
	return ""
}

// advModel returns a started model whose list holds withForm.
func advModel(t *testing.T, members func(context.Context, pokeapi.Kind, string) ([]int, error)) (*app.Model, uiQueue) {
	t.Helper()
	m, q := newModel(fakeSource{
		list:    func(context.Context) ([]pokeapi.Entry, error) { return withForm, nil },
		members: members,
	})
	t.Cleanup(m.Close)
	m.Start()
	q.run(t)
	return m, q
}

func names(es []pokeapi.Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

func TestAdvanced_FR7_ChoicesAreFixedTables(t *testing.T) {
	if n := len(app.Generations); n != 9 {
		t.Errorf("generations = %d, want 9", n)
	}
	if got := app.Generations[0].Label; got != "Gen 1 · Kanto" {
		t.Errorf("first generation = %q", got)
	}
	if got := app.Generations[8].Label; got != "Gen 9 · Paldea" {
		t.Errorf("last generation = %q", got)
	}
	if n := len(app.Types); n != 18 {
		t.Errorf("types = %d, want 18", n)
	}
	for _, c := range app.Types {
		if c.Key == "stellar" || c.Key == "shadow" || c.Key == "unknown" {
			t.Errorf("type %q must not be offered", c.Key)
		}
	}
	if n := len(app.EggGroups); n != 15 {
		t.Errorf("egg groups = %d, want 15", n)
	}
	if got := app.Labels(app.AnyType, app.Types); got[0] != "Any type" || len(got) != 19 {
		t.Errorf("type options = %v", got)
	}
}

func TestAdvanced_FR7_ClosedPanelIgnoresConditions(t *testing.T) {
	// Given conditions that are chosen but a panel that is closed
	m, _ := advModel(t, okMembers)
	m.Type = label(t, app.Types, "electric")

	// When the model syncs
	m.SyncAdvanced()

	// Then nothing is fetched and the list is as FR-1 says
	if m.FilterStatus != app.Idle {
		t.Errorf("FilterStatus = %v, want Idle", m.FilterStatus)
	}
	if got := len(m.Visible()); got != len(withForm) {
		t.Errorf("visible = %d, want %d", got, len(withForm))
	}
}

func TestAdvanced_FR7_ConditionsAreAnded(t *testing.T) {
	// Given a loaded list and an open panel
	m, q := advModel(t, okMembers)
	m.AdvancedOpen = true

	// When type Electric and Gen 1 are chosen
	m.Type = label(t, app.Types, "electric")
	m.Generation = label(t, app.Generations, "1")
	m.SyncAdvanced()
	if m.FilterStatus != app.Loading {
		t.Fatalf("FilterStatus = %v, want Loading", m.FilterStatus)
	}
	q.run(t)

	// Then only the Pokémon in both sets remain
	if m.FilterStatus != app.Ready {
		t.Fatalf("FilterStatus = %v, want Ready", m.FilterStatus)
	}
	if got := names(m.Visible()); !slices.Equal(got, []string{"pikachu"}) {
		t.Errorf("visible = %v", got)
	}

	// And the name query narrows them further (simple search AND advanced)
	m.Query = "zzz"
	if len(m.Visible()) != 0 || !m.NoResults() {
		t.Errorf("visible = %v, NoResults = %v", names(m.Visible()), m.NoResults())
	}
}

func TestAdvanced_FR7_TypeAloneKeepsFormRows(t *testing.T) {
	m, q := advModel(t, okMembers)
	m.AdvancedOpen = true

	m.Type = label(t, app.Types, "electric")
	m.SyncAdvanced()
	q.run(t)

	if got := names(m.Visible()); !slices.Equal(got, []string{"pikachu", "raichu", "pikachu-cosplay"}) {
		t.Errorf("visible = %v", got)
	}
}

func TestAdvanced_FR7_SpeciesConditionsDropFormRows(t *testing.T) {
	// The species sets hold species IDs, so a form's number (#10080) is never in them.
	for _, tt := range []struct {
		name string
		set  func(m *app.Model)
	}{
		{"egg group", func(m *app.Model) { m.EggGroup = label(t, app.EggGroups, "fairy") }},
		{"generation", func(m *app.Model) { m.Generation = label(t, app.Generations, "1") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, q := advModel(t, okMembers)
			m.AdvancedOpen = true

			tt.set(m)
			m.SyncAdvanced()
			q.run(t)

			for _, e := range m.Visible() {
				if e.ID >= 10000 {
					t.Errorf("form row %s is shown", e.Name)
				}
			}
			if len(m.Visible()) == 0 {
				t.Error("no rows")
			}
		})
	}
}

func TestAdvanced_FR7_ClearAndAnyRemoveTheConditions(t *testing.T) {
	m, q := advModel(t, okMembers)
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "electric")
	m.SyncAdvanced()
	q.run(t)
	m.Query = "pi"

	// When the conditions are cleared
	m.ClearAdvanced()
	m.SyncAdvanced()

	// Then the list is the name search again, and the query is kept
	if m.FilterStatus != app.Idle {
		t.Errorf("FilterStatus = %v, want Idle", m.FilterStatus)
	}
	if got := names(m.Visible()); !slices.Equal(got, []string{"pikachu", "pikachu-cosplay"}) {
		t.Errorf("visible = %v", got)
	}
	if m.Query != "pi" || m.Type != app.AnyType {
		t.Errorf("Query = %q, Type = %q", m.Query, m.Type)
	}

	// And choosing Any by hand does the same
	m.Type = label(t, app.Types, "electric")
	m.SyncAdvanced() // the set is cached: no fetch, no post
	if m.FilterStatus != app.Ready {
		t.Fatalf("FilterStatus = %v, want Ready", m.FilterStatus)
	}
	m.Type = app.AnyType
	m.SyncAdvanced()
	if m.FilterStatus != app.Idle {
		t.Errorf("after Any: FilterStatus = %v, want Idle", m.FilterStatus)
	}
}

func TestAdvanced_FR7_ClosingThePanelDropsTheConditionsButKeepsTheChoice(t *testing.T) {
	m, q := advModel(t, okMembers)
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "electric")
	m.SyncAdvanced()
	q.run(t)

	m.AdvancedOpen = false
	m.SyncAdvanced()

	if got := len(m.Visible()); got != len(withForm) {
		t.Errorf("visible = %d, want all %d", got, len(withForm))
	}
	if m.Type != label(t, app.Types, "electric") {
		t.Errorf("Type = %q, want the choice kept", m.Type)
	}
}

func TestAdvanced_FR3_FR7_FailureShowsErrorAndRetryRepeatsTheFetch(t *testing.T) {
	// Given an API that fails once
	var calls atomic.Int32
	m, q := advModel(t, func(ctx context.Context, k pokeapi.Kind, key string) ([]int, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("network is down")
		}
		return okMembers(ctx, k, key)
	})
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "electric")

	// When the fetch fails
	m.SyncAdvanced()
	q.run(t)

	// Then the status is Failed, and the list is not shown as empty
	if m.FilterStatus != app.Failed || m.FilterErr == nil {
		t.Fatalf("FilterStatus = %v, err = %v", m.FilterStatus, m.FilterErr)
	}
	if m.NoResults() {
		t.Error("NoResults while the filter failed")
	}

	// When Retry is pressed
	m.Retry()
	if m.FilterStatus != app.Loading {
		t.Fatalf("after Retry: FilterStatus = %v, want Loading", m.FilterStatus)
	}
	q.run(t)

	// Then the same fetch is repeated and succeeds
	if m.FilterStatus != app.Ready || len(m.Visible()) != 3 {
		t.Errorf("FilterStatus = %v, visible = %v", m.FilterStatus, names(m.Visible()))
	}
}

func TestAdvanced_FR3_FR7_NoMatchIsNoResults(t *testing.T) {
	m, q := advModel(t, func(context.Context, pokeapi.Kind, string) ([]int, error) { return []int{999}, nil })
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "fire")
	m.SyncAdvanced()
	if m.NoResults() {
		t.Error("NoResults while loading")
	}
	q.run(t)

	if !m.NoResults() {
		t.Errorf("want NoResults, visible = %v", names(m.Visible()))
	}
}

func TestAdvanced_FR4_FR7_ChangingTheConditionDiscardsThePreviousResult(t *testing.T) {
	// Given a fetch for Electric that is still running
	release := make(chan struct{})
	started := make(chan context.Context, 2)
	m, q := advModel(t, func(ctx context.Context, k pokeapi.Kind, key string) ([]int, error) {
		started <- ctx
		if key == "electric" {
			<-release
		}
		return okMembers(ctx, k, key)
	})
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "electric")
	m.SyncAdvanced()
	electricCtx := recvCtx(t, started)

	// When another condition is chosen
	m.Type = app.AnyType
	m.EggGroup = label(t, app.EggGroups, "fairy")
	m.SyncAdvanced()

	// Then the old fetch is canceled
	if electricCtx.Err() == nil {
		t.Error("the previous fetch was not canceled")
	}
	// And the new result is applied first, then the old one arrives late
	for m.FilterStatus == app.Loading {
		q.run(t)
	}
	close(release)
	q.run(t)

	// Then the late result of the old fetch never shows
	if got := names(m.Visible()); !slices.Equal(got, []string{"pikachu", "mr-mime"}) {
		t.Errorf("visible = %v, want the egg group only", got)
	}
}

func TestAdvanced_Fair_FR7_ASetIsNotFetchedTwice(t *testing.T) {
	var calls atomic.Int32
	m, q := advModel(t, func(ctx context.Context, k pokeapi.Kind, key string) ([]int, error) {
		calls.Add(1)
		return okMembers(ctx, k, key)
	})
	m.AdvancedOpen = true

	for _, key := range []string{"electric", "fire", "electric"} {
		m.Type = label(t, app.Types, key)
		m.SyncAdvanced()
		for m.FilterStatus == app.Loading {
			q.run(t)
		}
	}

	if n := calls.Load(); n != 2 {
		t.Errorf("fetches = %d, want 2 (electric once, fire once)", n)
	}
}

func TestAdvanced_FR4_FR7_CloseCancelsTheFetch(t *testing.T) {
	started := make(chan context.Context, 1)
	m, _ := advModel(t, func(ctx context.Context, _ pokeapi.Kind, _ string) ([]int, error) {
		started <- ctx
		<-ctx.Done()
		return nil, ctx.Err()
	})
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "electric")
	m.SyncAdvanced()
	ctx := recvCtx(t, started)

	m.Close()

	if ctx.Err() == nil {
		t.Error("the fetch was not canceled by Close")
	}
}

func TestAdvanced_FR4_FR7_AFailureOfACanceledFetchIsNotShown(t *testing.T) {
	// Given a type fetch that fails once it is canceled
	started := make(chan context.Context, 2)
	release := make(chan struct{})
	m, q := advModel(t, func(ctx context.Context, k pokeapi.Kind, key string) ([]int, error) {
		if key == "electric" {
			started <- ctx
			<-ctx.Done()
			<-release // fail only after the new result is in
			return nil, ctx.Err()
		}
		return okMembers(ctx, k, key)
	})
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "electric")
	m.SyncAdvanced()
	recvCtx(t, started)

	// When another condition replaces it
	m.Type = app.AnyType
	m.EggGroup = label(t, app.EggGroups, "fairy")
	m.SyncAdvanced()
	for m.FilterStatus == app.Loading {
		q.run(t)
	}

	close(release)
	q.run(t) // the canceled fetch's error arrives late

	// Then it does not replace the new result
	if m.FilterStatus != app.Ready || m.FilterErr != nil {
		t.Errorf("FilterStatus = %v, err = %v", m.FilterStatus, m.FilterErr)
	}
}

func TestAdvanced_FR7_RowsAreOnlyNarrowedByLoadedSets(t *testing.T) {
	// Given a set that has not arrived
	m, q := advModel(t, okMembers)
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "electric")
	m.SyncAdvanced()

	// Then the name search alone decides the rows, until the set arrives
	if got := len(m.Visible()); got != len(withForm) {
		t.Errorf("while loading: visible = %d, want %d", got, len(withForm))
	}
	rev := m.Rev()
	q.run(t)
	if got := len(m.Visible()); got != 3 {
		t.Errorf("after loading: visible = %d, want 3", got)
	}
	if m.Rev() == rev {
		t.Error("Rev did not change although the rows did")
	}
}

func TestAdvanced_FR7_NothingHappensAfterClose(t *testing.T) {
	m, q := advModel(t, okMembers)
	m.AdvancedOpen = true
	m.Type = label(t, app.Types, "electric")
	m.Close()

	m.SyncAdvanced()

	if m.FilterStatus != app.Idle {
		t.Errorf("FilterStatus = %v, want Idle", m.FilterStatus)
	}
	q.assertEmpty(t)
}
