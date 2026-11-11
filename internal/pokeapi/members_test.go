package pokeapi_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/naoya03708/pokedex-app/internal/pokeapi"
)

// The three endpoints behind the advanced search, in the shape PokeAPI uses.
const (
	typeJSON = `{"name":"electric","pokemon":[
 {"slot":1,"pokemon":{"name":"pikachu","url":"https://pokeapi.co/api/v2/pokemon/25/"}},
 {"slot":1,"pokemon":{"name":"raichu","url":"https://pokeapi.co/api/v2/pokemon/26/"}},
 {"slot":1,"pokemon":{"name":"pikachu-cosplay","url":"https://pokeapi.co/api/v2/pokemon/10080/"}}]}`
	generationJSON = `{"id":1,"pokemon_species":[
 {"name":"pikachu","url":"https://pokeapi.co/api/v2/pokemon-species/25/"},
 {"name":"bulbasaur","url":"https://pokeapi.co/api/v2/pokemon-species/1/"}]}`
	eggGroupJSON = `{"name":"fairy","pokemon_species":[
 {"name":"pikachu","url":"https://pokeapi.co/api/v2/pokemon-species/25/"},
 {"name":"mr-mime","url":"https://pokeapi.co/api/v2/pokemon-species/122/"}]}`
)

func membersAPI(t *testing.T) *fakeAPI {
	return newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/type/electric":
			w.Write([]byte(typeJSON))
		case "/generation/1":
			w.Write([]byte(generationJSON))
		case "/egg-group/fairy":
			w.Write([]byte(eggGroupJSON))
		default:
			http.NotFound(w, r)
		}
	})
}

func TestMembers_FR7_ReadsIDsFromEachEndpoint(t *testing.T) {
	tests := []struct {
		name string
		kind pokeapi.Kind
		key  string
		want []int
	}{
		{"type has Pokémon IDs, forms included", pokeapi.KindType, "electric", []int{25, 26, 10080}},
		{"generation has species IDs", pokeapi.KindGeneration, "1", []int{1, 25}},
		{"egg group has species IDs", pokeapi.KindEggGroup, "fairy", []int{25, 122}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given the API
			c := newClient(membersAPI(t), "", nil)

			// When the members are requested
			got, err := c.Members(context.Background(), tt.kind, tt.key)

			// Then the IDs come back in ascending order
			if err != nil || !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestMembers_FR7_SecondCallUsesMemoryCache(t *testing.T) {
	api := membersAPI(t)
	c := newClient(api, "", nil)

	c.Members(context.Background(), pokeapi.KindType, "electric")
	c.Members(context.Background(), pokeapi.KindType, "electric")

	if n := api.hits.Load(); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

func TestMembers_FR7_NextLaunchUsesDiskCache(t *testing.T) {
	dir := t.TempDir()
	api := membersAPI(t)
	newClient(api, dir, nil).Members(context.Background(), pokeapi.KindGeneration, "1")

	got, err := newClient(api, dir, nil).Members(context.Background(), pokeapi.KindGeneration, "1")

	if err != nil || !slices.Equal(got, []int{1, 25}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if n := api.hits.Load(); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

func TestMembers_FR7_RefetchesAfterSevenDays(t *testing.T) {
	dir := t.TempDir()
	api := membersAPI(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	newClient(api, dir, clock).Members(context.Background(), pokeapi.KindEggGroup, "fairy")

	now = now.Add(6 * 24 * time.Hour)
	newClient(api, dir, clock).Members(context.Background(), pokeapi.KindEggGroup, "fairy")
	if n := api.hits.Load(); n != 1 {
		t.Fatalf("after 6 days: requests = %d, want 1", n)
	}

	now = now.Add(2 * 24 * time.Hour)
	newClient(api, dir, clock).Members(context.Background(), pokeapi.KindEggGroup, "fairy")
	if n := api.hits.Load(); n != 2 {
		t.Fatalf("after 8 days: requests = %d, want 2", n)
	}
}

func TestMembers_FR7_ServesExpiredCacheWhenRefreshFails(t *testing.T) {
	// Given an expired cache and an API that is now down
	dir := t.TempDir()
	var down atomic.Bool
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(typeJSON))
	})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	newClient(api, dir, clock).Members(context.Background(), pokeapi.KindType, "electric")
	now = now.Add(30 * 24 * time.Hour)
	down.Store(true)

	// When the members are requested
	got, err := newClient(api, dir, clock).Members(context.Background(), pokeapi.KindType, "electric")

	// Then the stale set is used instead of an error
	if err != nil || len(got) != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestMembers_FR7_ErrorWhenNoCacheAndServerFails_NotCached(t *testing.T) {
	var down atomic.Bool
	down.Store(true)
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(typeJSON))
	})
	c := newClient(api, t.TempDir(), nil)

	if _, err := c.Members(context.Background(), pokeapi.KindType, "electric"); err == nil {
		t.Fatal("want an error while the API is down")
	}
	down.Store(false)
	got, err := c.Members(context.Background(), pokeapi.KindType, "electric")

	if err != nil || len(got) != 3 {
		t.Fatalf("after recovery: got %v, %v", got, err)
	}
}

func TestMembers_FR7_RejectsKeysThatCouldEscapeTheCacheDir(t *testing.T) {
	api := membersAPI(t)
	c := newClient(api, t.TempDir(), nil)

	for _, key := range []string{"", "../x", "a/b", "Electric", "fire?x=1"} {
		if _, err := c.Members(context.Background(), pokeapi.KindType, key); err == nil {
			t.Errorf("key %q: want an error", key)
		}
	}
	if _, err := c.Members(context.Background(), pokeapi.Kind("move"), "tackle"); err == nil {
		t.Error("unknown kind: want an error")
	}
	if n := api.hits.Load(); n != 0 {
		t.Errorf("requests = %d, want 0", n)
	}
}

func TestMembers_FR7_CorruptDiskCacheIsRefetched(t *testing.T) {
	dir := t.TempDir()
	api := membersAPI(t)
	newClient(api, dir, nil).Members(context.Background(), pokeapi.KindType, "electric")
	corruptAll(t, dir)

	got, err := newClient(api, dir, nil).Members(context.Background(), pokeapi.KindType, "electric")

	if err != nil || len(got) != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestMembers_FR4_CanceledContextStopsRequest(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
	})
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := newClient(api, "", nil).Members(ctx, pokeapi.KindType, "electric")
		done <- err
	}()
	<-started

	cancel()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "canceled") {
			t.Errorf("err = %v, want a cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request was not canceled")
	}
}
