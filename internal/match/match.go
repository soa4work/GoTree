// Package match scores how likely a person named in a record is a person
// already in the tree. Old records spell names freely (Meyer, Maier,
// Mayr; Schmidt, Schmitt), so names are compared by their Cologne
// phonetic code (Kölner Phonetik, made for German names; Russian and Kazakh
// names are transliterated first) and by edit
// distance; ages and life dates rule out the impossible.
package match

import (
	"strings"
	"unicode"
)

// Query is a person as a record describes them.
type Query struct {
	Given, Surname string
	// Sex is M, F or empty.
	Sex string
	// BirthYear is the estimated birth year, 0 when unknown.
	BirthYear int
	// RecordYear is the year of the record, 0 when unknown.
	RecordYear int
}

// Candidate is a person in the tree.
type Candidate struct {
	Given, Surname string
	Sex            string
	// BirthYear and DeathYear are 0 when unknown.
	BirthYear, DeathYear int
}

// Score is between 0 (no match) and 1 (same name, sex and birth year).
func Score(q Query, c Candidate) float64 {
	name := 0.5*nameSimilarity(q.Surname, c.Surname) + 0.5*givenSimilarity(q.Given, c.Given)
	year := 0.5
	if q.BirthYear != 0 && c.BirthYear != 0 {
		switch d := abs(q.BirthYear - c.BirthYear); {
		case d <= 1:
			year = 1
		case d <= 3:
			year = 0.8
		case d <= 5:
			year = 0.5
		case d <= 10:
			year = 0.2
		default:
			year = 0
		}
	}
	score := 0.75*name + 0.25*year
	if (q.Sex == "M" || q.Sex == "F") && (c.Sex == "M" || c.Sex == "F") && q.Sex != c.Sex {
		score *= 0.3
	}
	if q.RecordYear != 0 {
		// Dead well before the record, or born well after it. A burial can
		// be recorded the year after the death, a baptism before the
		// birth year is entered, hence the year of slack.
		if c.DeathYear != 0 && c.DeathYear < q.RecordYear-1 {
			score *= 0.2
		}
		if c.BirthYear != 0 && c.BirthYear > q.RecordYear+1 {
			score *= 0.2
		}
	}
	return score
}

// givenSimilarity compares given names word by word, so "Johann Georg"
// matches a "Georg" who was called by his second name.
func givenSimilarity(a, b string) float64 {
	wa, wb := strings.Fields(a), strings.Fields(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0.5
	}
	best := 0.0
	for _, x := range wa {
		for _, y := range wb {
			best = max(best, nameSimilarity(x, y))
		}
	}
	// Only a later name agreeing is a weaker sign than the first one.
	return (best + nameSimilarity(wa[0], wb[0])) / 2
}

func nameSimilarity(a, b string) float64 {
	na, nb := Normalize(a), Normalize(b)
	switch {
	case na == "" || nb == "":
		return 0.5
	case na == nb:
		return 1
	}
	ratio := 1 - float64(levenshtein(na, nb))/float64(max(len([]rune(na)), len([]rune(nb))))
	// Short codes collide (Johann and Anna are both "06"), so a shared
	// code only counts with the same first letter or two consonants.
	if code := Phonetic(na); code == Phonetic(nb) && (na[0] == nb[0] || len(strings.Trim(code, "0")) >= 2) {
		return max(ratio, 0.9)
	}
	return ratio
}

var folds = strings.NewReplacer(
	"ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
	"á", "a", "à", "a", "â", "a", "å", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ç", "c", "ñ", "n", "ÿ", "y", "š", "s", "ž", "z", "č", "c",
)

// translit maps Russian and Kazakh letters to a simple GOST/BGN-like Latin
// spelling close to how German and English records spell the same names, so
// the Cologne phonetic code (which only knows Latin letters) works on them:
// Мейер, Майер and Meyer all code alike, as do Иванов, Ivanov and Iwanow.
// The Kazakh letters fall back to their base letters.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh",
	'з': "z", 'и': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o",
	'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts",
	'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu",
	'я': "ya",
	// Kazakh
	'ә': "a", 'ғ': "g", 'қ': "k", 'ң': "n", 'ө': "o", 'ұ': "u", 'ү': "u", 'һ': "h", 'і': "i",
}

// Normalize lower-cases a name, transliterates Cyrillic, spells out umlauts
// and drops accents and everything that is not a letter.
func Normalize(s string) string {
	s = folds.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if t, ok := translit[r]; ok {
			b.WriteString(t)
		} else if unicode.IsLetter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Phonetic returns the Cologne phonetic code of a normalized name.
func Phonetic(s string) string {
	// Umlauts were spelled out as ae/oe/ue, which codes the same as a/o/u
	// since vowels are dropped after the first letter.
	r := []rune(strings.ToUpper(s))
	at := func(i int) rune {
		if i < 0 || i >= len(r) {
			return 0
		}
		return r[i]
	}
	in := func(c rune, set string) bool { return c != 0 && strings.ContainsRune(set, c) }
	var codes []byte
	for i, c := range r {
		prev, next := at(i-1), at(i+1)
		var code string
		switch {
		case in(c, "AEIJOUY"):
			code = "0"
		case c == 'H':
			continue
		case c == 'B':
			code = "1"
		case c == 'P':
			code = "1"
			if next == 'H' {
				code = "3"
			}
		case c == 'D' || c == 'T':
			code = "2"
			if in(next, "CSZ") {
				code = "8"
			}
		case in(c, "FVW"):
			code = "3"
		case in(c, "GKQ"):
			code = "4"
		case c == 'C':
			switch {
			case i == 0:
				code = "8"
				if in(next, "AHKLOQRUX") {
					code = "4"
				}
			case in(prev, "SZ"):
				code = "8"
			case in(next, "AHKOQUX"):
				code = "4"
			default:
				code = "8"
			}
		case c == 'X':
			code = "48"
			if in(prev, "CKQ") {
				code = "8"
			}
		case c == 'L':
			code = "5"
		case c == 'M' || c == 'N':
			code = "6"
		case c == 'R':
			code = "7"
		case c == 'S' || c == 'Z':
			code = "8"
		default:
			continue
		}
		codes = append(codes, code...)
	}
	var out []byte
	for i, c := range codes {
		if i > 0 && c == codes[i-1] {
			continue
		}
		if c == '0' && len(out) > 0 {
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
