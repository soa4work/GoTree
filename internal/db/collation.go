package db

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"fmt"
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
// apart and the Kazakh letters (ә ғ қ ң ө ұ ү һ і) landed after "я". This
// registers a collation backed by the Unicode (CLDR root) collation of
// golang.org/x/text. It puts every Kazakh letter right after its base letter,
// ё after е and i/ï/ü after the plain Latin letter, ignores case and, as
// NOCASE users expected for Latin names, accents ("Muller" sorts with
// "Müller").
//
// Each comparison runs a Go callback (about 1.5 µs) and SQLite cannot use an
// index for it, so `ORDER BY ... COLLATE NAMES` sorts every matching row
// before LIMIT applies. That is fine for small, bounded sets (sources,
// places, citations). The person list can hold tens of thousands of rows and
// is paged, so the store keeps NamesKey sort keys in persons.surname_key and
// persons.given_key with an index instead (#49).
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

// NamesKeyFunction is the SQL function names_key(text), NamesKey for
// migrations.
const NamesKeyFunction = "names_key"

// collators pools Collators: one keeps scratch buffers and is not safe for
// concurrent use, while SQLite calls the collation from every pooled
// connection.
var collators = &sync.Pool{New: func() any {
	return collate.New(language.Und, collate.IgnoreCase, collate.IgnoreDiacritics)
}}

// NamesKey is the sort key of s under the NAMES collation: comparing two keys
// bytewise orders them exactly as COLLATE NAMES orders the strings. It is
// never nil, so it can be bound to a NOT NULL BLOB column.
//
// Any change to sortKeyText, cyrillicWeights, the collator options or a
// golang.org/x/text upgrade that changes keys needs a new migration that
// recomputes persons.surname_key and persons.given_key with names_key(...);
// old and new keys mixed sort wrongly. TestNamesKeyGolden catches this.
func NamesKey(s string) []byte {
	c := collators.Get().(*collate.Collator)
	defer collators.Put(c)
	var buf collate.Buffer
	k := c.KeyFromString(&buf, sortKeyText(s))
	// Copy: k aliases buf.
	return append(make([]byte, 0, len(k)), k...)
}

// keyFingerprintSample covers every Cyrillic alphabet letter, ё/й, accented
// Latin letters, ß and the empty string.
var keyFingerprintSample = func() []string {
	out := []string{"", "ё", "й", "Ё", "Й", "é", "è", "ü", "Ü", "ñ", "ç", "ø", "å", "ß", "Müller", "Straße", "Ёлкин"}
	for _, r := range cyrillicAlphabet {
		out = append(out, string(r), strings.ToUpper(string(r)))
	}
	return out
}()

// NamesKeyFingerprint hashes NamesKey over a fixed sample of strings. Open
// compares it with the one stored in app_meta and recomputes the persons'
// stored keys when it differs (see refreshNameKeys).
func NamesKeyFingerprint() string {
	h := sha256.New()
	for _, s := range keyFingerprintSample {
		k := NamesKey(s)
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(k)))
		h.Write(n[:])
		h.Write(k)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func init() {
	sqlite.MustRegisterCollationUtf8(NamesCollation, func(a, b string) int {
		c := collators.Get().(*collate.Collator)
		defer collators.Put(c)
		return c.CompareString(sortKeyText(a), sortKeyText(b))
	})
	sqlite.MustRegisterDeterministicScalarFunction(NamesKeyFunction, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			switch v := args[0].(type) {
			case nil:
				return []byte{}, nil
			case string:
				return NamesKey(v), nil
			case []byte:
				return NamesKey(string(v)), nil
			default:
				return NamesKey(fmt.Sprint(v)), nil
			}
		})
}
