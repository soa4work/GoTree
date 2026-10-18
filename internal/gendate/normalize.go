package gendate

import (
	"regexp"
	"strings"
)

// keywordAliases maps the words people type for qualifiers, in English
// and German, to GEDCOM keywords.
var keywordAliases = map[string]string{
	"abt": "ABT", "about": "ABT", "ca": "ABT", "circa": "ABT", "c": "ABT", "approx": "ABT", "um": "ABT", "etwa": "ABT", "gegen": "ABT",
	"cal": "CAL", "calculated": "CAL", "errechnet": "CAL",
	"est": "EST", "estimated": "EST", "geschätzt": "EST",
	"bef": "BEF", "before": "BEF", "vor": "BEF",
	"aft": "AFT", "after": "AFT", "nach": "AFT",
	"bet": "BET", "between": "BET", "zwischen": "BET",
	"and": "AND", "und": "AND",
	"from": "FROM", "von": "FROM", "seit": "FROM",
	"to": "TO", "bis": "TO",

	// Russian.
	"около": "ABT", "ок": "ABT", "примерно": "ABT", "приблизительно": "ABT", "прибл": "ABT", "приб": "ABT",
	"расч": "CAL", "расчетно": "CAL",
	"оценка": "EST", "оценочно": "EST",
	"до": "BEF", "после": "AFT", "между": "BET", "и": "AND", "с": "FROM", "по": "TO",

	// Kazakh. The postpositions (дейін, кейін, аралығында) come after the
	// date and are handled by rewriteKazakh.
	"шамамен": "ABT", "шам": "ABT",
}

// noiseWords are year markers that carry no information once the date is
// read: "1850 г.", "1850 года", "в 1850 году", "1850 ж.", "1850 жылы".
var noiseWords = map[string]bool{
	"г": true, "гг": true, "год": true, "года": true, "году": true, "годов": true, "годами": true, "годах": true,
	"ж": true, "жж": true, "жыл": true, "жылы": true, "жылдың": true,
}

var monthAliases = map[string]string{
	"january": "JAN", "januar": "JAN", "jänner": "JAN", "jan": "JAN",
	"february": "FEB", "februar": "FEB", "feb": "FEB",
	"march": "MAR", "märz": "MAR", "maerz": "MAR", "mrz": "MAR", "mar": "MAR", "mär": "MAR",
	"april": "APR", "apr": "APR",
	"may": "MAY", "mai": "MAY",
	"june": "JUN", "juni": "JUN", "jun": "JUN",
	"july": "JUL", "juli": "JUL", "jul": "JUL",
	"august": "AUG", "aug": "AUG",
	"september": "SEP", "sept": "SEP", "sep": "SEP",
	"october": "OCT", "oktober": "OCT", "oct": "OCT", "okt": "OCT",
	"november": "NOV", "nov": "NOV",
	"december": "DEC", "dezember": "DEC", "dec": "DEC", "dez": "DEC",
}

func init() {
	// Russian: nominative, genitive and abbreviations (ё is folded to е).
	for mon, words := range map[string][]string{
		"JAN": {"январь", "января", "январе", "янв"},
		"FEB": {"февраль", "февраля", "феврале", "февр", "фев"},
		"MAR": {"март", "марта", "марте", "мар"},
		"APR": {"апрель", "апреля", "апреле", "апр"},
		"MAY": {"май", "мая", "мае"},
		"JUN": {"июнь", "июня", "июне", "июн"},
		"JUL": {"июль", "июля", "июле", "июл"},
		"AUG": {"август", "августа", "августе", "авг"},
		"SEP": {"сентябрь", "сентября", "сентябре", "сент", "сен"},
		"OCT": {"октябрь", "октября", "октябре", "окт"},
		"NOV": {"ноябрь", "ноября", "ноябре", "нояб", "ноя"},
		"DEC": {"декабрь", "декабря", "декабре", "дек"},
	} {
		for _, w := range words {
			monthAliases[w] = mon
		}
	}
	// Kazakh: the bare month name plus the locative and genitive forms
	// that show up in "12 наурызда" and "наурыздың" in longer phrases.
	for mon, stem := range map[string]string{
		"JAN": "қаңтар", "FEB": "ақпан", "MAR": "наурыз", "APR": "сәуір",
		"MAY": "мамыр", "JUN": "маусым", "JUL": "шілде", "AUG": "тамыз",
		"SEP": "қыркүйек", "OCT": "қазан", "NOV": "қараша", "DEC": "желтоқсан",
	} {
		monthAliases[stem] = mon
		for _, suf := range kazakhCaseSuffixes {
			monthAliases[stem+suf] = mon
		}
	}
}

// kazakhCaseSuffixes are the locative (-да/-де/-та/-те) and genitive
// (-дың/-дің/-тың/-тің/-ның/-нің) endings tried after a month name.
var kazakhCaseSuffixes = []string{"да", "де", "та", "те", "дың", "дің", "тың", "тің", "ның", "нің"}

var (
	yearRange = regexp.MustCompile(`^(\d{3,4})\s*[-–]\s*(\d{3,4})(?:\s*(?:гг?|жж?)\.?)?$`)
	ordinal   = regexp.MustCompile(`^(\d{1,2})(st|nd|rd|th)$`)
)

// Normalize reads dates the way people write them when no program forces
// GEDCOM syntax: "ca. 1850", "March 12, 1850", "12. März 1850",
// "1850-1860", "vor 1900", "1850?". It returns what Parse returns for
// anything Parse already accepts.
func Normalize(s string) (Date, error) {
	if d, err := Parse(s); err == nil {
		return d, nil
	}
	rewritten, ok := rewriteLoose(s)
	if !ok {
		return Parse(s)
	}
	return Parse(rewritten)
}

func rewriteLoose(s string) (string, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.NewReplacer("ё", "е").Replace(s)
	if strings.HasPrefix(s, "(") {
		return "", false
	}
	if m := yearRange.FindStringSubmatch(s); m != nil {
		return "BET " + m[1] + " AND " + m[2], true
	}
	uncertain := strings.HasSuffix(s, "?")
	s = strings.TrimSuffix(s, "?")
	s = strings.NewReplacer(",", " ", " - ", " TO ", "–", " TO ").Replace(s)

	var toks []string
	for _, tok := range strings.Fields(s) {
		// "12." and "ca." lose their dot, but "12.3.1850" keeps it for Parse.
		if strings.Count(tok, ".") == 1 && strings.HasSuffix(tok, ".") {
			tok = strings.TrimSuffix(tok, ".")
		}
		toks = append(toks, tok)
	}
	// "в 1850 году": the leading preposition is dropped.
	if len(toks) > 1 && toks[0] == "в" {
		toks = toks[1:]
	}
	toks = rewriteKazakh(toks)

	var out []string
	hasFrom := false
	for _, tok := range toks {
		switch {
		case noiseWords[tok]:
			continue
		case tok == "до" && hasFrom:
			tok = "TO" // "с 1850 до 1860"
		case keywordAliases[tok] != "":
			tok = keywordAliases[tok]
		case monthAliases[tok] != "":
			tok = monthAliases[tok]
		case ordinal.MatchString(tok):
			tok = ordinal.FindStringSubmatch(tok)[1]
		}
		if tok == "FROM" {
			hasFrom = true
		}
		out = append(out, tok)
	}
	if len(out) == 0 {
		return "", false
	}
	out = monthFirstToDayFirst(out)
	if uncertain && !isQualifier(out[0]) {
		out = append([]string{"ABT"}, out...)
	}
	return strings.Join(out, " "), true
}

// rewriteKazakh turns the Kazakh postpositions into leading GEDCOM
// keywords: "1900 жылға дейін" -> BEF, "1900 жылдан кейін" -> AFT,
// "1850 және 1860 аралығында" -> BET … AND, "1850 жылдан 1860 жылға
// дейін" -> FROM … TO. "1850 жылы 12 наурызда" becomes "12 наурызда 1850".
// Tokens without such a postposition are returned unchanged.
func rewriteKazakh(toks []string) []string {
	n := len(toks)
	if n >= 3 && toks[1] == "жылы" && isYear(toks[0]) {
		return append(append([]string{}, toks[2:]...), toks[0])
	}
	if n < 2 {
		return toks
	}
	head := toks[:n-1]
	switch toks[n-1] {
	case "аралығында":
		for i, t := range head {
			if t == "және" {
				return concat([]string{"BET"}, trimYearCase(head[:i]), []string{"AND"}, trimYearCase(head[i+1:]))
			}
		}
	case "дейін":
		for i, t := range head {
			if t == "жылдан" {
				return concat([]string{"FROM"}, head[:i], []string{"TO"}, trimYearCase(head[i+1:]))
			}
		}
		return concat([]string{"BEF"}, trimYearCase(head))
	case "кейін":
		return concat([]string{"AFT"}, trimYearCase(head))
	case "бұрын":
		return concat([]string{"BEF"}, trimYearCase(head))
	case "бастап", "жылдан":
		return concat([]string{"FROM"}, trimYearCase(head))
	}
	// A bare "1900 жылға" is left as is so that it is rejected.
	return toks
}

// trimYearCase drops a trailing ablative or dative year marker
// ("1850 жылдан", "1900 жылға") that belongs to the postposition.
func trimYearCase(toks []string) []string {
	if n := len(toks); n > 0 && (toks[n-1] == "жылдан" || toks[n-1] == "жылға") {
		return toks[:n-1]
	}
	return toks
}

func concat(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func isYear(s string) bool {
	return len(s) >= 3 && len(s) <= 4 && isDigits(s)
}

// monthFirstToDayFirst turns the English "MAR 12 1850" into "12 MAR 1850"
// wherever it occurs, e.g. on both sides of BET … AND.
func monthFirstToDayFirst(toks []string) []string {
	for i := 0; i+2 < len(toks); i++ {
		if monthNumber(toks[i]) != 0 && len(toks[i+1]) <= 2 && isDigits(toks[i+1]) && isDigits(toks[i+2]) {
			toks[i], toks[i+1] = toks[i+1], toks[i]
		}
	}
	return toks
}

func isQualifier(tok string) bool {
	switch tok {
	case "ABT", "CAL", "EST", "BEF", "AFT", "BET", "FROM", "TO", "INT":
		return true
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
