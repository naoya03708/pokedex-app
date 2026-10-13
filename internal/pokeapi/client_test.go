package pokeapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/naoya03708/pokedex-app/internal/pokeapi"
)

const listJSON = `{"count":3,"results":[
 {"name":"pikachu","url":"https://pokeapi.co/api/v2/pokemon/25/"},
 {"name":"bulbasaur","url":"https://pokeapi.co/api/v2/pokemon/1/"},
 {"name":"mr-mime","url":"https://pokeapi.co/api/v2/pokemon/122/"}]}`

const pikachuJSON = `{"id":25,"name":"pikachu",
 "types":[{"slot":1,"type":{"name":"electric"}}],
 "stats":[
  {"base_stat":35,"stat":{"name":"hp"}},
  {"base_stat":55,"stat":{"name":"attack"}},
  {"base_stat":40,"stat":{"name":"defense"}},
  {"base_stat":50,"stat":{"name":"special-attack"}},
  {"base_stat":50,"stat":{"name":"special-defense"}},
  {"base_stat":90,"stat":{"name":"speed"}}],
 "sprites":{"front_default":"https://img.example/25.png"}}`

// fakeAPI serves the two PokeAPI endpoints and counts requests per path.
type fakeAPI struct {
	srv  *httptest.Server
	hits atomic.Int32
}

func newFakeAPI(t *testing.T, h http.HandlerFunc) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	if h == nil {
		h = func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/pokemon":
				w.Write([]byte(listJSON))
			case "/pokemon/pikachu":
				w.Write([]byte(pikachuJSON))
			default:
				http.NotFound(w, r)
			}
		}
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func newClient(f *fakeAPI, dir string, now func() time.Time) *pokeapi.Client {
	return pokeapi.New(pokeapi.Options{BaseURL: f.srv.URL, CacheDir: dir, Now: now})
}

func TestList_FR1_SortedByNumberWithIDsFromURL(t *testing.T) {
	// Given an API that returns names out of order
	c := newClient(newFakeAPI(t, nil), t.TempDir(), nil)

	// When the list is fetched
	got, err := c.List(context.Background())

	// Then entries are ordered by Pokédex number, with IDs parsed from the URLs
	if err != nil {
		t.Fatal(err)
	}
	want := []pokeapi.Entry{{ID: 1, Name: "bulbasaur"}, {ID: 25, Name: "pikachu"}, {ID: 122, Name: "mr-mime"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestList_FR1_SecondCallUsesMemoryCache(t *testing.T) {
	api := newFakeAPI(t, nil)
	c := newClient(api, t.TempDir(), nil)

	c.List(context.Background())
	c.List(context.Background())

	if n := api.hits.Load(); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
}

func TestList_FR1_NextLaunchUsesDiskCache(t *testing.T) {
	// Given a list fetched by a previous launch (same cache dir)
	dir := t.TempDir()
	api := newFakeAPI(t, nil)
	newClient(api, dir, nil).List(context.Background())

	// When a new client (new launch) asks for the list
	got, err := newClient(api, dir, nil).List(context.Background())

	// Then it is served from disk without another request
	if err != nil || len(got) != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
	if n := api.hits.Load(); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
}

func TestList_FR1_RefetchesAfterSevenDays(t *testing.T) {
	dir := t.TempDir()
	api := newFakeAPI(t, nil)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	newClient(api, dir, clock).List(context.Background())

	now = now.Add(6 * 24 * time.Hour)
	newClient(api, dir, clock).List(context.Background())
	if n := api.hits.Load(); n != 1 {
		t.Fatalf("after 6 days: requests = %d, want 1", n)
	}

	now = now.Add(2 * 24 * time.Hour)
	newClient(api, dir, clock).List(context.Background())
	if n := api.hits.Load(); n != 2 {
		t.Fatalf("after 8 days: requests = %d, want 2", n)
	}
}

func TestList_Offline_ServesExpiredCacheWhenFetchFails(t *testing.T) {
	// Given an expired cache and an API that is now down
	dir := t.TempDir()
	var down atomic.Bool
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(listJSON))
	})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	newClient(api, dir, clock).List(context.Background())
	now = now.Add(30 * 24 * time.Hour)
	down.Store(true)

	// When the list is requested
	got, err := newClient(api, dir, clock).List(context.Background())

	// Then the stale list is shown instead of an error
	if err != nil || len(got) != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestList_FR3_ErrorWhenNoCacheAndServerFails(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newClient(api, t.TempDir(), nil)

	_, err := c.List(context.Background())

	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want one mentioning status 500", err)
	}
}

func TestList_FR3_FailureIsNotCached(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		w.Write([]byte(listJSON))
	})
	c := newClient(api, t.TempDir(), nil)
	c.List(context.Background())
	fail.Store(false)

	// When Retry is pressed (same call again)
	got, err := c.List(context.Background())

	if err != nil || len(got) != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestList_FR4_CanceledContextStopsRequest(t *testing.T) {
	release := make(chan struct{})
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(func() { close(release) })
	c := newClient(api, t.TempDir(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { _, err := c.List(ctx); done <- err }()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("List did not return after cancel")
	}
}

func TestGet_FR2_ParsesTypesStatsAndSprite(t *testing.T) {
	c := newClient(newFakeAPI(t, nil), t.TempDir(), nil)

	p, err := c.Get(context.Background(), "pikachu")

	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 25 || p.Name != "pikachu" || p.SpriteURL != "https://img.example/25.png" {
		t.Fatalf("basic fields: %+v", p)
	}
	if len(p.Types) != 1 || p.Types[0] != "electric" {
		t.Fatalf("types = %v", p.Types)
	}
	want := []pokeapi.Stat{
		{Name: "hp", Base: 35}, {Name: "attack", Base: 55}, {Name: "defense", Base: 40},
		{Name: "special-attack", Base: 50}, {Name: "special-defense", Base: 50}, {Name: "speed", Base: 90},
	}
	if len(p.Stats) != len(want) {
		t.Fatalf("stats = %v", p.Stats)
	}
	for i := range want {
		if p.Stats[i] != want[i] {
			t.Fatalf("stat %d = %v, want %v", i, p.Stats[i], want[i])
		}
	}
}

func TestGet_FR2_TypesOrderedBySlot(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":6,"name":"charizard","stats":[],"sprites":{},
		 "types":[{"slot":2,"type":{"name":"flying"}},{"slot":1,"type":{"name":"fire"}}]}`))
	})
	c := newClient(api, t.TempDir(), nil)

	p, err := c.Get(context.Background(), "charizard")

	if err != nil {
		t.Fatal(err)
	}
	if len(p.Types) != 2 || p.Types[0] != "fire" || p.Types[1] != "flying" {
		t.Fatalf("types = %v, want [fire flying]", p.Types)
	}
}

func TestGet_Cache_NeverExpires(t *testing.T) {
	dir := t.TempDir()
	api := newFakeAPI(t, nil)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	newClient(api, dir, clock).Get(context.Background(), "pikachu")

	now = now.AddDate(2, 0, 0)
	p, err := newClient(api, dir, clock).Get(context.Background(), "pikachu")

	if err != nil || p.ID != 25 {
		t.Fatalf("got %+v, %v", p, err)
	}
	if n := api.hits.Load(); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
}

func TestGet_FR3_NotFoundIsReported(t *testing.T) {
	c := newClient(newFakeAPI(t, nil), t.TempDir(), nil)

	_, err := c.Get(context.Background(), "missingno")

	if !errors.Is(err, pokeapi.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGet_RejectsNamesThatCouldEscapeTheCacheDir(t *testing.T) {
	api := newFakeAPI(t, nil)
	c := newClient(api, t.TempDir(), nil)

	for _, name := range []string{"", "../x", "a/b", `a\b`, "pika chu", "pikachu?x=1"} {
		if _, err := c.Get(context.Background(), name); err == nil {
			t.Errorf("Get(%q) succeeded, want error", name)
		}
	}
	if n := api.hits.Load(); n != 0 {
		t.Fatalf("requests = %d, want 0", n)
	}
}

func TestGet_NameIsCaseInsensitive(t *testing.T) {
	c := newClient(newFakeAPI(t, nil), t.TempDir(), nil)

	p, err := c.Get(context.Background(), "Pikachu")

	if err != nil || p.Name != "pikachu" {
		t.Fatalf("got %+v, %v", p, err)
	}
}

func TestGet_CorruptDiskCacheIsRefetched(t *testing.T) {
	dir := t.TempDir()
	api := newFakeAPI(t, nil)
	newClient(api, dir, nil).Get(context.Background(), "pikachu")
	corruptAll(t, dir)

	p, err := newClient(api, dir, nil).Get(context.Background(), "pikachu")

	if err != nil || p.ID != 25 {
		t.Fatalf("got %+v, %v", p, err)
	}
	if n := api.hits.Load(); n != 2 {
		t.Fatalf("requests = %d, want 2", n)
	}
}

func TestNew_NFR_DefaultHTTPTimeoutIs10Seconds(t *testing.T) {
	if got := pokeapi.DefaultTimeout; got != 10*time.Second {
		t.Fatalf("DefaultTimeout = %v, want 10s", got)
	}
}

func TestNew_NFR_NoCacheDirStillWorks(t *testing.T) {
	c := newClient(newFakeAPI(t, nil), "", nil)

	if _, err := c.List(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestGet_FR4_CanceledContextStopsRequest(t *testing.T) {
	release := make(chan struct{})
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(func() { close(release) })
	c := newClient(api, t.TempDir(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { _, err := c.Get(ctx, "pikachu"); done <- err }()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Get did not return after cancel")
	}
}

func TestList_FR1_FutureTimestampIsNotFresh(t *testing.T) {
	dir := t.TempDir()
	api := newFakeAPI(t, nil)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newClient(api, dir, func() time.Time { return now.Add(365 * 24 * time.Hour) }).List(context.Background())

	newClient(api, dir, func() time.Time { return now }).List(context.Background())

	if n := api.hits.Load(); n != 2 {
		t.Fatalf("requests = %d, want 2", n)
	}
}

func TestClient_ConcurrentUseIsRaceFree(t *testing.T) {
	c := newClient(newFakeAPI(t, nil), t.TempDir(), nil)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() { defer wg.Done(); c.List(context.Background()) }()
		go func() { defer wg.Done(); c.Get(context.Background(), "pikachu") }()
	}
	wg.Wait()
}
