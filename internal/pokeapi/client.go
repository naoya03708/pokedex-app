// Package pokeapi is a small PokeAPI client with timeouts, context support
// and a memory + disk cache.
package pokeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultTimeout is the HTTP timeout used when Options.HTTPClient is nil.
	DefaultTimeout = 10 * time.Second
	// ListTTL is how long the cached name list is considered fresh.
	ListTTL = 7 * 24 * time.Hour

	defaultBaseURL = "https://pokeapi.co/api/v2"
	maxBodyBytes   = 16 << 20
)

// ErrNotFound is returned by Get when the API has no such Pokémon.
var ErrNotFound = errors.New("pokeapi: not found")

// validName limits names to what PokeAPI uses, which also keeps them safe to
// use as cache file names and URL path segments.
var validName = regexp.MustCompile(`^[a-z0-9-]+$`)

// Entry is one row of the name list.
type Entry struct {
	ID   int
	Name string
}

// Stat is one base stat, e.g. {"hp", 35}.
type Stat struct {
	Name string
	Base int
}

// Pokemon is the detail shown in the detail pane.
type Pokemon struct {
	ID        int
	Name      string
	Types     []string // primary type first
	Stats     []Stat   // in API order: hp, attack, defense, special-attack, special-defense, speed
	SpriteURL string
}

// Options configures a Client. Zero values select production defaults.
type Options struct {
	BaseURL    string           // default https://pokeapi.co/api/v2
	CacheDir   string           // empty disables the disk cache
	HTTPClient *http.Client     // default: DefaultTimeout
	Now        func() time.Time // default time.Now
}

// DefaultCacheDir returns os.UserCacheDir()/pokedex.
func DefaultCacheDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "pokedex"), nil
}

// Client talks to PokeAPI and caches what it fetched. It is safe for
// concurrent use.
type Client struct {
	base string
	dir  string
	hc   *http.Client
	now  func() time.Time

	mu      sync.Mutex
	list    *listCache
	details map[string]*Pokemon
	sprites map[int][]byte
}

type listCache struct {
	FetchedAt time.Time
	Entries   []Entry
}

// fresh reports whether the list is within ListTTL. A FetchedAt in the future
// (clock skew, edited file) is not fresh.
func (l *listCache) fresh(now time.Time) bool {
	age := now.Sub(l.FetchedAt)
	return age >= 0 && age < ListTTL
}

// New returns a Client.
func New(o Options) *Client {
	c := &Client{
		base:    strings.TrimRight(o.BaseURL, "/"),
		dir:     o.CacheDir,
		hc:      o.HTTPClient,
		now:     o.Now,
		details: map[string]*Pokemon{},
		sprites: map[int][]byte{},
	}
	if c.base == "" {
		c.base = defaultBaseURL
	}
	if c.hc == nil {
		c.hc = &http.Client{Timeout: DefaultTimeout}
	}
	if c.now == nil {
		c.now = time.Now
	}
	return c
}

// List returns all Pokémon names ordered by Pokédex number. A fresh cache
// (memory, then disk) is used without a request. If the cache is older than
// ListTTL it is refreshed, but when the refresh fails the stale list is
// returned instead of an error, so the app keeps working offline. A canceled
// context always returns the context's error. The returned slice is shared
// with the cache and must not be modified.
func (c *Client) List(ctx context.Context) ([]Entry, error) {
	cached := c.loadList()
	if cached != nil && cached.fresh(c.now()) {
		return cached.Entries, nil
	}

	entries, err := c.fetchList(ctx)
	if err != nil {
		if cached != nil && ctx.Err() == nil {
			return cached.Entries, nil
		}
		return nil, err
	}
	fresh := &listCache{FetchedAt: c.now(), Entries: entries}
	c.mu.Lock()
	c.list = fresh
	c.mu.Unlock()
	c.writeCache("list.json", fresh)
	return entries, nil
}

func (c *Client) loadList() *listCache {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.list == nil {
		var lc listCache
		if c.readCache("list.json", &lc) && len(lc.Entries) > 0 {
			c.list = &lc
		}
	}
	return c.list
}

func (c *Client) fetchList(ctx context.Context) ([]Entry, error) {
	var body struct {
		Results []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"results"`
	}
	if err := c.getJSON(ctx, c.base+"/pokemon?limit=100000", &body); err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(body.Results))
	for _, r := range body.Results {
		id, ok := idFromURL(r.URL)
		if !ok || r.Name == "" {
			continue
		}
		entries = append(entries, Entry{ID: id, Name: r.Name})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries, nil
}

// idFromURL extracts 25 from ".../pokemon/25/".
func idFromURL(u string) (int, bool) {
	u = strings.TrimRight(u, "/")
	id, err := strconv.Atoi(u[strings.LastIndex(u, "/")+1:])
	return id, err == nil
}

// Get returns the detail of one Pokémon. Details are cached without expiry.
// A name PokeAPI does not know returns ErrNotFound. The returned value is
// shared with the cache and must not be modified.
func (c *Client) Get(ctx context.Context, name string) (*Pokemon, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !validName.MatchString(name) {
		return nil, fmt.Errorf("pokeapi: invalid name %q", name)
	}

	c.mu.Lock()
	p, ok := c.details[name]
	c.mu.Unlock()
	if ok {
		return p, nil
	}

	file := filepath.Join("pokemon", name+".json")
	var cached Pokemon
	if c.readCache(file, &cached) && cached.Name != "" {
		p = &cached
	} else {
		var err error
		if p, err = c.fetchPokemon(ctx, name); err != nil {
			return nil, err
		}
		c.writeCache(file, p)
	}

	c.mu.Lock()
	c.details[name] = p
	c.mu.Unlock()
	return p, nil
}

func (c *Client) fetchPokemon(ctx context.Context, name string) (*Pokemon, error) {
	var body struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Types []struct {
			Slot int `json:"slot"`
			Type struct {
				Name string `json:"name"`
			} `json:"type"`
		} `json:"types"`
		Stats []struct {
			Base int `json:"base_stat"`
			Stat struct {
				Name string `json:"name"`
			} `json:"stat"`
		} `json:"stats"`
		Sprites struct {
			FrontDefault string `json:"front_default"`
		} `json:"sprites"`
	}
	if err := c.getJSON(ctx, c.base+"/pokemon/"+name, &body); err != nil {
		return nil, err
	}

	sort.SliceStable(body.Types, func(i, j int) bool { return body.Types[i].Slot < body.Types[j].Slot })
	p := &Pokemon{ID: body.ID, Name: body.Name, SpriteURL: body.Sprites.FrontDefault}
	for _, t := range body.Types {
		p.Types = append(p.Types, t.Type.Name)
	}
	for _, s := range body.Stats {
		p.Stats = append(p.Stats, Stat{Name: s.Stat.Name, Base: s.Base})
	}
	return p, nil
}

func (c *Client) getJSON(ctx context.Context, url string, v any) error {
	b, err := c.fetch(ctx, url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("pokeapi: decode %s: %w", url, err)
	}
	return nil
}

// fetch GETs url and returns the body. 404 is ErrNotFound.
func (c *Client) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pokeapi: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("pokeapi: GET %s: unexpected status %d", req.URL.Path, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("pokeapi: read %s: %w", req.URL.Path, err)
	}
	if len(b) > maxBodyBytes {
		return nil, fmt.Errorf("pokeapi: GET %s: response larger than %d bytes", req.URL.Path, maxBodyBytes)
	}
	return b, nil
}

// readCache loads a JSON cache file. Missing or corrupt files report false.
func (c *Client) readCache(rel string, v any) bool {
	b, ok := c.readFile(rel)
	return ok && json.Unmarshal(b, v) == nil
}

// writeCache stores v as JSON. See writeFile.
func (c *Client) writeCache(rel string, v any) {
	if b, err := json.Marshal(v); err == nil {
		c.writeFile(rel, b)
	}
}

// readFile loads a non-empty cache file.
func (c *Client) readFile(rel string) ([]byte, bool) {
	if c.dir == "" {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(c.dir, rel))
	return b, err == nil && len(b) > 0
}

// writeFile stores b atomically. The cache is best effort: failures are
// ignored because the data is still served from memory.
func (c *Client) writeFile(rel string, b []byte) {
	if c.dir == "" {
		return
	}
	path := filepath.Join(c.dir, rel)
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		os.Remove(tmp.Name())
	}
}

func isPNG(b []byte) bool { return bytes.HasPrefix(b, pngMagic) }

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// Sprite returns the sprite image (PNG bytes) of p, cached without expiry
// under its Pokédex number. A Pokémon without a sprite URL is ErrNotFound.
// The returned bytes are shared with the cache and must not be modified.
func (c *Client) Sprite(ctx context.Context, p *Pokemon) ([]byte, error) {
	if p == nil || p.SpriteURL == "" {
		return nil, ErrNotFound
	}
	if p.ID <= 0 {
		return nil, fmt.Errorf("pokeapi: invalid Pokémon number %d", p.ID)
	}
	if u, err := url.Parse(p.SpriteURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("pokeapi: unsupported sprite URL %q", p.SpriteURL)
	}

	c.mu.Lock()
	b, ok := c.sprites[p.ID]
	c.mu.Unlock()
	if ok {
		return b, nil
	}

	file := filepath.Join("sprites", strconv.Itoa(p.ID)+".png")
	if b, ok = c.readFile(file); !ok || !isPNG(b) {
		var err error
		if b, err = c.fetch(ctx, p.SpriteURL); err != nil {
			return nil, err
		}
		// Do not keep a captive-portal page or a truncated file forever.
		if !isPNG(b) {
			return nil, fmt.Errorf("pokeapi: sprite %s is not a PNG image", p.SpriteURL)
		}
		c.writeFile(file, b)
	}

	c.mu.Lock()
	c.sprites[p.ID] = b
	c.mu.Unlock()
	return b, nil
}
