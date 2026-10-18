package db

import (
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
	"modernc.org/sqlite"
)

// NamesCollation is the SQLite collation to ORDER BY names, surnames, places
// and titles: `ORDER BY surname COLLATE NAMES`.
//
// SQLite's built-in NOCASE folds only ASCII, so "иван" and "Иван" sorted
// apart and the Kazakh letters (ә ғ қ ң ө ұ ү һ і) landed after "я". Instead
// of a precomputed sort-key column (a migration, a reindex and a write path
// to keep in sync for every sortable text column) this registers a collation
// backed by the Unicode (CLDR root) collation of golang.org/x/text. It puts
// every Kazakh letter right after its base letter, ё after е and i/ï/ü after
// the plain Latin letter, ignores case and, as NOCASE users expected for
// Latin names, accents ("Muller" sorts with "Müller"). Lists are paged
// (at most a few hundred rows) or sorted once, so the callback cost is fine.
//
// The root collation orders Cyrillic wrongly for Russian and Kazakh (й is
// folded into и with the accents, ү sorts before ұ, і right after и), so
// Cyrillic letters are first replaced by private-use runes whose code points
// follow the Russian/Kazakh alphabet (see cyrillicWeights) and the collator
// sorts those after every Latin letter. ё deliberately shares the weight of
// е (conventional). Cyrillic letters outside the alphabet (ў, ґ, ...) keep
// their root order, before the mapped ones.
const NamesCollation = "NAMES"

// cyrillicAlphabet is the Kazakh Cyrillic alphabet, which contains the Russian
// one in its usual order (й right after и).
const cyrillicAlphabet = "аәбвгғдеёжзийкқлмнңоөпрстуұүфхһцчшщъыіьэюя"

// cyrillicWeights maps a lower-case letter of cyrillicAlphabet to a
// private-use rune ordered like the alphabet.
var cyrillicWeights = func() map[rune]rune {
	m := map[rune]rune{}
	w := rune(0xE000)
	for _, r := range cyrillicAlphabet {
		if r == 'ё' {
			m[r] = m['е']
			continue
		}
		m[r] = w
		w++
	}
	return m
}()

// sortKeyText lower-cases s, composes it (so й is one rune) and replaces the
// Cyrillic alphabet letters by their sort weights.
func sortKeyText(s string) string {
	s = norm.NFC.String(s)
	return strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		if w, ok := cyrillicWeights[r]; ok {
			return w
		}
		return r
	}, s)
}

func init() {
	// A Collator keeps scratch buffers and is not safe for concurrent use,
	// while SQLite calls the collation from every pooled connection.
	pool := &sync.Pool{New: func() any {
		return collate.New(language.Und, collate.IgnoreCase, collate.IgnoreDiacritics)
	}}
	sqlite.MustRegisterCollationUtf8(NamesCollation, func(a, b string) int {
		c := pool.Get().(*collate.Collator)
		defer pool.Put(c)
		return c.CompareString(sortKeyText(a), sortKeyText(b))
	})
}
