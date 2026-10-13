package app_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/naoya03708/pokedex-app/internal/app"
	"github.com/naoya03708/pokedex-app/internal/pokeapi"
)

var entries = []pokeapi.Entry{
	{ID: 1, Name: "bulbasaur"}, {ID: 25, Name: "pikachu"}, {ID: 26, Name: "raichu"}, {ID: 122, Name: "mr-mime"},
}

// fakeSource lets each test decide what List and Get do.
type fakeSource struct {
	list func(ctx context.Context) ([]pokeapi.Entry, error)
	get  func(ctx context.Context, name string) (*pokeapi.Pokemon, error)
}

func (f fakeSource) List(ctx context.Context) ([]pokeapi.Entry, error) { return f.list(ctx) }
func (f fakeSource) Get(ctx context.Context, n string) (*pokeapi.Pokemon, error) {
	return f.get(ctx, n)
}

func okList(context.Context) ([]pokeapi.Entry, error) { return entries, nil }
func okGet(_ context.Context, n string) (*pokeapi.Pokemon, error) {
	return &pokeapi.Pokemon{ID: 1, Name: n}, nil
}

// uiQueue stands in for Window.Update: posted functions run only when the
// test (acting as the UI thread) asks for them.
type uiQueue chan func()

func newQueue() uiQueue { return make(uiQueue, 16) }

func (q uiQueue) Post(fn func()) { q <- fn }

// run executes the next posted function.
func (q uiQueue) run(t *testing.T) {
	t.Helper()
	select {
	case fn := <-q:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was posted to the UI thread")
	}
}

func (q uiQueue) assertEmpty(t *testing.T) {
	t.Helper()
	select {
	case <-q:
		t.Fatal("unexpected post to the UI thread")
	case <-time.After(50 * time.Millisecond):
	}
}

func recvCtx(t *testing.T, ch <-chan context.Context) context.Context {
	t.Helper()
	select {
	case ctx := <-ch:
		return ctx
	case <-time.After(2 * time.Second):
		t.Fatal("fetch did not start")
		return nil
	}
}

func newModel(src fakeSource) (*app.Model, uiQueue) {
	q := newQueue()
	return app.New(src, q.Post), q
}

func TestFilter_FR1_CaseInsensitiveSubstring(t *testing.T) {
	got := app.Filter(entries, "PIKA")

	if len(got) != 1 || got[0].Name != "pikachu" {
		t.Fatalf("got %v", got)
	}
}

func TestFilter_FR1_MatchesInsideName(t *testing.T) {
	got := app.Filter(entries, "chu")

	if len(got) != 2 || got[0].Name != "pikachu" || got[1].Name != "raichu" {
		t.Fatalf("got %v, want pikachu then raichu in number order", got)
	}
}

func TestFilter_FR1_EmptyQueryReturnsAllInOrder(t *testing.T) {
	for _, q := range []string{"", "   "} {
		got := app.Filter(entries, q)

		if len(got) != len(entries) {
			t.Fatalf("query %q: got %d entries, want %d", q, len(got), len(entries))
		}
		for i := range entries {
			if got[i] != entries[i] {
				t.Fatalf("query %q: order changed at %d", q, i)
			}
		}
	}
}

func TestFilter_FR3_NoMatchIsEmpty(t *testing.T) {
	if got := app.Filter(entries, "zzz"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestModel_FR3_ListShowsLoadingThenReady(t *testing.T) {
	// Given a model that has not started
	m, q := newModel(fakeSource{list: okList})
	if m.ListStatus != app.Idle {
		t.Fatalf("initial status = %v", m.ListStatus)
	}

	// When it starts
	m.Start()

	// Then it is loading until the result is applied on the UI thread
	if m.ListStatus != app.Loading {
		t.Fatalf("after Start: %v, want Loading", m.ListStatus)
	}
	q.run(t)
	if m.ListStatus != app.Ready || len(m.Visible()) != 4 {
		t.Fatalf("after result: %v, %d visible", m.ListStatus, len(m.Visible()))
	}
}

func TestModel_FR3_ListErrorThenRetryReloads(t *testing.T) {
	var calls atomic.Int32
	m, q := newModel(fakeSource{list: func(context.Context) ([]pokeapi.Entry, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("offline")
		}
		return entries, nil
	}})
	m.Start()
	q.run(t)
	if m.ListStatus != app.Failed || m.ListErr == nil {
		t.Fatalf("status = %v, err = %v", m.ListStatus, m.ListErr)
	}

	m.Retry()

	if m.ListStatus != app.Loading || m.ListErr != nil {
		t.Fatalf("after Retry: %v, err %v", m.ListStatus, m.ListErr)
	}
	q.run(t)
	if m.ListStatus != app.Ready {
		t.Fatalf("after retry result: %v", m.ListStatus)
	}
}

func TestModel_FR1_VisibleFollowsQueryAndNoResults(t *testing.T) {
	m, q := newModel(fakeSource{list: okList})
	m.Start()
	q.run(t)

	m.Query = "RAI"
	if v := m.Visible(); len(v) != 1 || v[0].Name != "raichu" {
		t.Fatalf("visible = %v", v)
	}
	if m.NoResults() {
		t.Fatal("NoResults with a match")
	}

	m.Query = "zzz"
	if len(m.Visible()) != 0 || !m.NoResults() {
		t.Fatalf("visible = %v, NoResults = %v", m.Visible(), m.NoResults())
	}

	m.Query = ""
	if len(m.Visible()) != 4 {
		t.Fatalf("visible = %d, want all", len(m.Visible()))
	}
}

func TestModel_FR3_NoResultsIsFalseWhileLoadingOrFailed(t *testing.T) {
	m, q := newModel(fakeSource{list: func(context.Context) ([]pokeapi.Entry, error) {
		return nil, errors.New("offline")
	}})
	m.Query = "x"
	m.Start()
	if m.NoResults() {
		t.Fatal("NoResults while loading")
	}
	q.run(t)
	if m.NoResults() {
		t.Fatal("NoResults after failure")
	}
}

func TestModel_FR2_SelectLoadsDetail(t *testing.T) {
	m, q := newModel(fakeSource{list: okList, get: okGet})

	m.Select("pikachu")

	if m.Selected != "pikachu" || m.DetailStatus != app.Loading || m.Detail != nil {
		t.Fatalf("after Select: %q %v %v", m.Selected, m.DetailStatus, m.Detail)
	}
	q.run(t)
	if m.DetailStatus != app.Ready || m.Detail == nil || m.Detail.Name != "pikachu" {
		t.Fatalf("after result: %v %+v", m.DetailStatus, m.Detail)
	}
}

func TestModel_FR3_DetailErrorThenRetryRefetches(t *testing.T) {
	var calls atomic.Int32
	m, q := newModel(fakeSource{list: okList, get: func(_ context.Context, n string) (*pokeapi.Pokemon, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("offline")
		}
		return &pokeapi.Pokemon{Name: n}, nil
	}})
	m.Select("pikachu")
	q.run(t)
	if m.DetailStatus != app.Failed || m.DetailErr == nil {
		t.Fatalf("status = %v, err = %v", m.DetailStatus, m.DetailErr)
	}

	m.Retry()

	if m.DetailStatus != app.Loading {
		t.Fatalf("after Retry: %v", m.DetailStatus)
	}
	q.run(t)
	if m.DetailStatus != app.Ready || m.Detail.Name != "pikachu" {
		t.Fatalf("after retry result: %v %+v", m.DetailStatus, m.Detail)
	}
}

func TestModel_FR4_StaleDetailResultIsDiscarded(t *testing.T) {
	// Given two fetches that ignore cancellation and finish on command
	release := map[string]chan struct{}{"pikachu": make(chan struct{}), "raichu": make(chan struct{})}
	m, q := newModel(fakeSource{list: okList, get: func(_ context.Context, n string) (*pokeapi.Pokemon, error) {
		<-release[n]
		return &pokeapi.Pokemon{Name: n}, nil
	}})
	m.Select("pikachu")
	m.Select("raichu")

	// When the first (now stale) fetch finishes before the second
	close(release["pikachu"])
	q.run(t)

	// Then its result is not shown
	if m.Detail != nil || m.DetailStatus != app.Loading || m.Selected != "raichu" {
		t.Fatalf("stale result applied: %q %v %+v", m.Selected, m.DetailStatus, m.Detail)
	}
	close(release["raichu"])
	q.run(t)
	if m.Detail == nil || m.Detail.Name != "raichu" || m.DetailStatus != app.Ready {
		t.Fatalf("final: %v %+v", m.DetailStatus, m.Detail)
	}
}

func TestModel_FR4_StaleDetailErrorIsDiscarded(t *testing.T) {
	release := map[string]chan struct{}{"pikachu": make(chan struct{}), "raichu": make(chan struct{})}
	m, q := newModel(fakeSource{list: okList, get: func(_ context.Context, n string) (*pokeapi.Pokemon, error) {
		<-release[n]
		if n == "pikachu" {
			return nil, errors.New("late failure")
		}
		return &pokeapi.Pokemon{Name: n}, nil
	}})
	m.Select("pikachu")
	m.Select("raichu")

	close(release["pikachu"])
	q.run(t)

	if m.DetailStatus == app.Failed || m.DetailErr != nil {
		t.Fatalf("stale error shown: %v %v", m.DetailStatus, m.DetailErr)
	}
	close(release["raichu"])
	q.run(t)
}

func TestModel_FR4_SelectingAnotherRowCancelsPreviousFetch(t *testing.T) {
	canceled := make(chan struct{})
	m, q := newModel(fakeSource{list: okList, get: func(ctx context.Context, n string) (*pokeapi.Pokemon, error) {
		if n == "pikachu" {
			<-ctx.Done()
			close(canceled)
			return nil, ctx.Err()
		}
		return &pokeapi.Pokemon{Name: n}, nil
	}})
	m.Select("pikachu")

	m.Select("raichu")

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("previous fetch was not canceled")
	}
	q.run(t)
	q.run(t)
	if m.Detail == nil || m.Detail.Name != "raichu" {
		t.Fatalf("detail = %+v", m.Detail)
	}
}

func TestModel_FR4_ReselectingLoadedPokemonDoesNotRefetch(t *testing.T) {
	var calls atomic.Int32
	m, q := newModel(fakeSource{list: okList, get: func(_ context.Context, n string) (*pokeapi.Pokemon, error) {
		calls.Add(1)
		return &pokeapi.Pokemon{Name: n}, nil
	}})
	m.Select("pikachu")
	q.run(t)

	m.Select("pikachu")

	q.assertEmpty(t)
	if calls.Load() != 1 || m.DetailStatus != app.Ready || m.Detail == nil {
		t.Fatalf("calls = %d, status = %v", calls.Load(), m.DetailStatus)
	}
}

func TestModel_FR4_CloseCancelsRunningFetchesAndIgnoresResults(t *testing.T) {
	// Given a list fetch and a detail fetch in flight
	listCtx := make(chan context.Context, 1)
	getCtx := make(chan context.Context, 1)
	finish := make(chan struct{})
	m, q := newModel(fakeSource{
		list: func(ctx context.Context) ([]pokeapi.Entry, error) {
			listCtx <- ctx
			<-finish
			return entries, nil
		},
		get: func(ctx context.Context, n string) (*pokeapi.Pokemon, error) {
			getCtx <- ctx
			<-finish
			return &pokeapi.Pokemon{Name: n}, nil
		},
	})
	m.Start()
	m.Select("pikachu")
	lc, gc := recvCtx(t, listCtx), recvCtx(t, getCtx)

	// When the window closes
	m.Close()

	// Then both contexts are canceled, and results that still arrive are ignored
	if lc.Err() == nil || gc.Err() == nil {
		t.Fatalf("contexts not canceled: list=%v get=%v", lc.Err(), gc.Err())
	}
	close(finish)
	for range 2 {
		q.run(t)
	}
	if m.ListStatus == app.Ready || m.DetailStatus == app.Ready {
		t.Fatalf("results applied after Close: %v %v", m.ListStatus, m.DetailStatus)
	}
}

func TestModel_FR4_CloseIsIdempotentAndBlocksNewWork(t *testing.T) {
	m, q := newModel(fakeSource{list: okList, get: okGet})
	m.Close()
	m.Close()

	m.Start()
	m.Select("pikachu")

	q.assertEmpty(t)
}
