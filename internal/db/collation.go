package db

import (
	"sync"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
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
const NamesCollation = "NAMES"

func init() {
	// A Collator keeps scratch buffers and is not safe for concurrent use,
	// while SQLite calls the collation from every pooled connection.
	pool := &sync.Pool{New: func() any {
		return collate.New(language.Und, collate.IgnoreCase, collate.IgnoreDiacritics)
	}}
	sqlite.MustRegisterCollationUtf8(NamesCollation, func(a, b string) int {
		c := pool.Get().(*collate.Collator)
		defer pool.Put(c)
		return c.CompareString(a, b)
	})
}
