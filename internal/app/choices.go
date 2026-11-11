package app

// The choices that switch a condition of the advanced search off. Each has its
// own text, so a closed drop-down says which condition it is.
const (
	AnyGeneration = "Any generation"
	AnyType       = "Any type"
	AnyEggGroup   = "Any egg group"
)

// Choice is one option of an advanced-search condition: the text the user
// sees and the key PokeAPI knows it by.
type Choice struct{ Label, Key string }

// Labels returns the options of a drop-down: any first, then the labels of cs.
func Labels(any string, cs []Choice) []string {
	out := []string{any}
	for _, c := range cs {
		out = append(out, c.Label)
	}
	return out
}

// Generations are the nine main-series generations, named with their region.
var Generations = []Choice{
	{"Gen 1 · Kanto", "1"}, {"Gen 2 · Johto", "2"}, {"Gen 3 · Hoenn", "3"},
	{"Gen 4 · Sinnoh", "4"}, {"Gen 5 · Unova", "5"}, {"Gen 6 · Kalos", "6"},
	{"Gen 7 · Alola", "7"}, {"Gen 8 · Galar", "8"}, {"Gen 9 · Paldea", "9"},
}

// Types are the 18 types a Pokémon can have (not stellar, shadow or unknown).
var Types = []Choice{
	{"Normal", "normal"}, {"Fire", "fire"}, {"Water", "water"}, {"Electric", "electric"},
	{"Grass", "grass"}, {"Ice", "ice"}, {"Fighting", "fighting"}, {"Poison", "poison"},
	{"Ground", "ground"}, {"Flying", "flying"}, {"Psychic", "psychic"}, {"Bug", "bug"},
	{"Rock", "rock"}, {"Ghost", "ghost"}, {"Dragon", "dragon"}, {"Dark", "dark"},
	{"Steel", "steel"}, {"Fairy", "fairy"},
}

// EggGroups are the 15 egg groups, with the PokeAPI names as keys.
var EggGroups = []Choice{
	{"Monster", "monster"}, {"Water 1", "water1"}, {"Bug", "bug"}, {"Flying", "flying"},
	{"Field", "ground"}, {"Fairy", "fairy"}, {"Grass", "plant"}, {"Human-Like", "humanshape"},
	{"Water 3", "water3"}, {"Mineral", "mineral"}, {"Amorphous", "indeterminate"},
	{"Water 2", "water2"}, {"Ditto", "ditto"}, {"Dragon", "dragon"}, {"Undiscovered", "no-eggs"},
}
