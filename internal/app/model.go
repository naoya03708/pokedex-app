// Package app holds the UI-independent state of the Pokédex: the search
// query, the filtered list, the selection and the loading state of the
// asynchronous fetches.
package app

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/naoya03708/pokedex-app/internal/pokeapi"
)

// Source is the API the model needs. *pokeapi.Client implements it.
type Source interface {
	List(ctx context.Context) ([]pokeapi.Entry, error)
	Get(ctx context.Context, name string) (*pokeapi.Pokemon, error)
	Sprite(ctx context.Context, p *pokeapi.Pokemon) ([]byte, error)
	Description(ctx context.Context, p *pokeapi.Pokemon) (string, error)
	SpriteByID(ctx context.Context, id int) ([]byte, error)
	Members(ctx context.Context, kind pokeapi.Kind, key string) ([]int, error)
}

// maxThumbFetches is how many row sprites are fetched at the same time.
const maxThumbFetches = 4

// Status is the state of one asynchronous fetch.
type Status int

const (
	Idle Status = iota
	Loading
	Ready
	Failed
)

// Poster runs fn on the UI thread. In production it is Window.Update.
type Poster func(fn func())

// Model is the state behind the window. All methods and fields must be used
// on the UI thread only; background work reports back through the Poster.
type Model struct {
	Query string // bound to the search box

	// The advanced search: the panel and the label of each condition, bound to
	// the widgets. The Any… labels (or "") mean no condition. While the panel is
	// closed all conditions are off, but the choices are kept.
	AdvancedOpen               bool
	Generation, Type, EggGroup string
	FilterStatus               Status // of the sets the conditions need
	FilterErr                  error

	ListStatus Status
	ListErr    error

	Selected     string
	Detail       *pokeapi.Pokemon
	DetailStatus Status
	DetailErr    error

	Sprite       []byte // PNG of the selected Pokémon, once SpriteStatus is Ready
	SpriteStatus Status

	// Description is the Pokédex text of the selected Pokémon, once DescStatus
	// is Ready. Failed (or a Pokémon with no text) means the UI shows nothing.
	Description string
	DescStatus  Status

	src    Source
	post   Poster
	ctx    context.Context // canceled by Close
	cancel context.CancelFunc
	closed bool

	// thumbs are the sprites of the result rows, by Pokédex number. A row that
	// failed stays Failed: it is not fetched again in this session.
	thumbs    map[int]thumb
	thumbSlot chan struct{} // holds one token per running thumb fetch

	entries   []pokeapi.Entry
	visible   []pokeapi.Entry // the rows matching Query and the loaded conditions, valid if visOK
	visQuery  string
	visOK     bool
	visRev    int // counts the times visible was recomputed
	detailGen int

	// The advanced search. applied are the conditions in force, sets the ID
	// sets fetched so far (kept for the session: a set is never fetched twice),
	// and filterGen tells the running fetch from an older one.
	applied      []cond
	sets         map[cond]map[int]struct{}
	filterGen    int
	cancelFilter context.CancelFunc
	// cancelDetail cancels the running detail fetch, if any.
	cancelDetail context.CancelFunc
}

// New returns a Model that fetches from src and publishes through post.
func New(src Source, post Poster) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	return &Model{
		Generation: AnyGeneration, Type: AnyType, EggGroup: AnyEggGroup,
		src: src, post: post, ctx: ctx, cancel: cancel,
		sets: map[cond]map[int]struct{}{}, thumbs: map[int]thumb{}, thumbSlot: make(chan struct{}, maxThumbFetches),
	}
}

type thumb struct {
	status Status
	png    []byte
}

// Thumb returns the sprite (PNG) of a result row and its state. The bytes are
// shared and must not be modified.
func (m *Model) Thumb(id int) ([]byte, Status) {
	t := m.thumbs[id]
	return t.png, t.status
}

// RequestThumb starts fetching the sprite of a result row, unless it is
// already loading, loaded or failed, so the view may call it in every frame.
// At most maxThumbFetches run at once.
func (m *Model) RequestThumb(id int) {
	if m.closed || m.thumbs[id].status != Idle {
		return
	}
	m.thumbs[id] = thumb{status: Loading}
	src, post, ctx, slot := m.src, m.post, m.ctx, m.thumbSlot
	go func() {
		select {
		case slot <- struct{}{}:
		case <-ctx.Done():
			return
		}
		img, err := src.SpriteByID(ctx, id)
		<-slot
		post(func() {
			if m.closed {
				return
			}
			if err != nil {
				m.thumbs[id] = thumb{status: Failed}
				return
			}
			m.thumbs[id] = thumb{status: Ready, png: img}
		})
	}()
}

// Start begins loading the name list. It does nothing while a load is
// already running or after Close.
func (m *Model) Start() {
	if m.closed || m.ListStatus == Loading {
		return
	}
	m.ListStatus, m.ListErr = Loading, nil
	src, post, ctx := m.src, m.post, m.ctx
	go func() {
		entries, err := src.List(ctx)
		post(func() {
			if m.closed {
				return
			}
			if err != nil {
				m.ListStatus, m.ListErr = Failed, err
				return
			}
			m.ListStatus = Ready
			m.entries, m.visOK = entries, false
		})
	}()
}

// Retry repeats the fetch that failed: the list, then the advanced search's
// sets, otherwise the selected detail.
func (m *Model) Retry() {
	switch {
	case m.ListStatus == Failed:
		m.Start()
	case m.FilterStatus == Failed:
		m.fetchFilters()
	case m.DetailStatus == Failed:
		m.fetchDetail(m.Selected)
	}
}

// Select shows the detail of the named Pokémon. A fetch for another row
// still running is canceled and its result discarded. Selecting the row that
// is already shown or loading does nothing.
func (m *Model) Select(name string) {
	if m.closed || name == "" {
		return
	}
	if name == m.Selected && (m.DetailStatus == Loading || m.DetailStatus == Ready) {
		return
	}
	m.fetchDetail(name)
}

func (m *Model) fetchDetail(name string) {
	if m.cancelDetail != nil {
		m.cancelDetail()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelDetail = cancel
	m.detailGen++
	m.Selected, m.Detail, m.DetailErr, m.DetailStatus = name, nil, nil, Loading
	m.Sprite, m.SpriteStatus = nil, Idle
	m.Description, m.DescStatus = "", Idle

	gen, src, post := m.detailGen, m.src, m.post
	go func() {
		defer cancel() // release the child context once the fetches are over
		p, err := src.Get(ctx, name)
		withSprite := err == nil && p.SpriteURL != ""
		withDesc := err == nil && p.SpeciesID > 0
		post(func() {
			if m.closed || gen != m.detailGen {
				return
			}
			if err != nil {
				m.DetailStatus, m.DetailErr = Failed, err
				return
			}
			m.DetailStatus, m.Detail = Ready, p
			if withDesc {
				m.DescStatus = Loading
			}
			m.SpriteStatus = Loading
			if !withSprite {
				m.SpriteStatus = Failed // nothing to show: the UI draws a placeholder
			}
		})
		// The description is fetched beside the sprite. A failure shows nothing.
		var wg sync.WaitGroup
		defer wg.Wait() // keep ctx alive until it is done
		if withDesc {
			wg.Add(1)
			go func() {
				defer wg.Done()
				text, descErr := src.Description(ctx, p)
				post(func() {
					if m.closed || gen != m.detailGen {
						return
					}
					if descErr != nil {
						m.DescStatus = Failed
						return
					}
					m.DescStatus, m.Description = Ready, text
				})
			}()
		}
		if !withSprite {
			return
		}
		// The text is already on screen; the image follows when it arrives.
		img, imgErr := src.Sprite(ctx, p)
		post(func() {
			if m.closed || gen != m.detailGen {
				return
			}
			if imgErr != nil {
				m.SpriteStatus = Failed
				return
			}
			m.SpriteStatus, m.Sprite = Ready, img
		})
	}()
}

// Close cancels every running fetch and ignores their results. The window
// calls it when it closes.
func (m *Model) Close() {
	m.closed = true
	m.cancel()
}

// cond is one condition of the advanced search: the members of a resource.
type cond struct {
	kind pokeapi.Kind
	key  string
}

// wanted returns the conditions the bound fields ask for. A closed panel asks
// for none.
func (m *Model) wanted() []cond {
	if !m.AdvancedOpen {
		return nil
	}
	var out []cond
	for _, f := range []struct {
		kind  pokeapi.Kind
		label string
		cs    []Choice
	}{
		{pokeapi.KindGeneration, m.Generation, Generations},
		{pokeapi.KindType, m.Type, Types},
		{pokeapi.KindEggGroup, m.EggGroup, EggGroups},
	} {
		for _, c := range f.cs {
			if c.Label == f.label {
				out = append(out, cond{f.kind, c.Key})
				break
			}
		}
	}
	return out
}

// SyncAdvanced applies the conditions chosen in the bound fields: it fetches
// the sets it does not have yet, and the rows follow. It does nothing when the
// conditions are as they were, so the view may call it in every frame.
func (m *Model) SyncAdvanced() {
	if m.closed {
		return
	}
	if want := m.wanted(); !slices.Equal(want, m.applied) {
		m.applied = want
		m.fetchFilters()
	}
}

// ClearAdvanced sets the three conditions to their Any… choice. The name query stays.
func (m *Model) ClearAdvanced() {
	m.Generation, m.Type, m.EggGroup = AnyGeneration, AnyType, AnyEggGroup
}

// fetchFilters fetches the sets of the applied conditions that are missing.
// An older fetch is canceled and its result discarded.
func (m *Model) fetchFilters() {
	if m.cancelFilter != nil {
		m.cancelFilter()
		m.cancelFilter = nil
	}
	m.filterGen++
	m.visOK = false
	m.FilterErr = nil

	var missing []cond
	for _, c := range m.applied {
		if _, ok := m.sets[c]; !ok {
			missing = append(missing, c)
		}
	}
	if len(missing) == 0 {
		m.FilterStatus = Idle
		if len(m.applied) > 0 {
			m.FilterStatus = Ready
		}
		return
	}
	m.FilterStatus = Loading

	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelFilter = cancel
	gen, src, post := m.filterGen, m.src, m.post
	go func() {
		defer cancel()
		got := make(map[cond]map[int]struct{}, len(missing))
		for _, c := range missing {
			ids, err := src.Members(ctx, c.kind, c.key)
			if err != nil {
				post(func() {
					if m.closed || gen != m.filterGen {
						return
					}
					m.FilterStatus, m.FilterErr = Failed, err
				})
				return
			}
			set := make(map[int]struct{}, len(ids))
			for _, id := range ids {
				set[id] = struct{}{}
			}
			got[c] = set
		}
		post(func() {
			if m.closed || gen != m.filterGen {
				return
			}
			for c, set := range got {
				m.sets[c] = set
			}
			m.FilterStatus, m.visOK = Ready, false
		})
	}()
}

// Visible returns the entries matching Query, in Pokédex-number order. The
// result is shared and must not be modified.
func (m *Model) Visible() []pokeapi.Entry {
	if !m.visOK || m.visQuery != m.Query {
		m.visible, m.visQuery, m.visOK = m.match(), m.Query, true
		m.visRev++
	}
	return m.visible
}

// Rev changes whenever Visible returns different rows, which lets the view
// notice a new set of rows even if their number is the same.
func (m *Model) Rev() int {
	m.Visible()
	return m.visRev
}

// match is the name search AND every applied condition whose set is loaded.
// An entry is in a set by its number: a type lists Pokémon, a generation and an
// egg group list species, whose IDs are the numbers of the default forms only.
func (m *Model) match() []pokeapi.Entry {
	rows := Filter(m.entries, m.Query)
	for _, c := range m.applied {
		set, ok := m.sets[c]
		if !ok {
			continue
		}
		kept := make([]pokeapi.Entry, 0, len(rows))
		for _, e := range rows {
			if _, in := set[e.ID]; in {
				kept = append(kept, e)
			}
		}
		rows = kept
	}
	return rows
}

// NoResults reports whether the list loaded, the conditions are in force, and
// nothing matches.
func (m *Model) NoResults() bool {
	return m.ListStatus == Ready && m.FilterStatus != Loading && m.FilterStatus != Failed &&
		len(m.Visible()) == 0
}

// Filter returns the entries whose name contains query, ignoring case and
// surrounding spaces, keeping their order. An empty query matches all.
func Filter(entries []pokeapi.Entry, query string) []pokeapi.Entry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return entries
	}
	var out []pokeapi.Entry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name), q) {
			out = append(out, e)
		}
	}
	return out
}
