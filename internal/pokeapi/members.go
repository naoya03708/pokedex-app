package pokeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

// Kind is a kind of resource whose members the advanced search filters by.
type Kind string

const (
	KindType       Kind = "type"
	KindGeneration Kind = "generation"
	KindEggGroup   Kind = "egg-group"
)

// memberField is the JSON field listing the members of a resource. A type
// lists Pokémon (forms included); a generation and an egg group list species.
func (k Kind) memberField() (string, bool) {
	switch k {
	case KindType:
		return "pokemon", true
	case KindGeneration, KindEggGroup:
		return "pokemon_species", true
	}
	return "", false
}

type membersCache struct {
	FetchedAt time.Time
	IDs       []int
}

// fresh reports whether the set is within ListTTL, like the name list.
func (m *membersCache) fresh(now time.Time) bool {
	age := now.Sub(m.FetchedAt)
	return age >= 0 && age < ListTTL
}

// Members returns, in ascending order, the IDs of the members of one resource:
// the Pokémon of a type (forms included), or the species of a generation or an
// egg group. A Pokémon's number is its species' ID except for forms, so one ID
// set serves every kind. Sets are cached in memory and on disk like the name
// list: a fresh one needs no request, and when refreshing an expired one fails
// the stale set is returned. The returned slice is shared with the cache and
// must not be modified.
func (c *Client) Members(ctx context.Context, kind Kind, key string) ([]int, error) {
	field, ok := kind.memberField()
	if !ok {
		return nil, fmt.Errorf("pokeapi: unknown kind %q", kind)
	}
	if !validName.MatchString(key) {
		return nil, fmt.Errorf("pokeapi: invalid key %q", key)
	}

	id := string(kind) + "-" + key
	c.mu.Lock()
	cached := c.members[id]
	if cached == nil {
		var mc membersCache
		if c.readCache(filepath.Join("filters", id+".json"), &mc) && len(mc.IDs) > 0 {
			cached = &mc
			c.members[id] = cached
		}
	}
	c.mu.Unlock()
	if cached != nil && cached.fresh(c.now()) {
		return cached.IDs, nil
	}

	ids, err := c.fetchMembers(ctx, string(kind)+"/"+key, field)
	if err != nil {
		if cached != nil && ctx.Err() == nil {
			return cached.IDs, nil
		}
		return nil, err
	}
	fresh := &membersCache{FetchedAt: c.now(), IDs: ids}
	c.mu.Lock()
	c.members[id] = fresh
	c.mu.Unlock()
	c.writeCache(filepath.Join("filters", id+".json"), fresh)
	return ids, nil
}

func (c *Client) fetchMembers(ctx context.Context, path, field string) ([]int, error) {
	// "pokemon" entries wrap the resource ({"pokemon": {"url": …}}); the
	// species lists hold it directly ({"url": …}).
	var body map[string]json.RawMessage
	if err := c.getJSON(ctx, c.base+"/"+path, &body); err != nil {
		return nil, err
	}
	var members []struct {
		URL     string `json:"url"`
		Pokemon struct {
			URL string `json:"url"`
		} `json:"pokemon"`
	}
	if err := json.Unmarshal(body[field], &members); err != nil {
		return nil, fmt.Errorf("pokeapi: decode %s: %w", path, err)
	}
	ids := make([]int, 0, len(members))
	for _, m := range members {
		u := m.URL
		if field == "pokemon" {
			u = m.Pokemon.URL
		}
		if id, ok := idFromURL(u); ok {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids, nil
}
