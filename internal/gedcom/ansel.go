package gedcom

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// anselChars maps the ANSEL (ANSI Z39.47) characters above 0x7F, with the
// additions the GEDCOM 5.5.1 specification makes, to Unicode.
var anselChars = map[byte]rune{
	0xA1: 'Ł', 0xA2: 'Ø', 0xA3: 'Đ', 0xA4: 'Þ', 0xA5: 'Æ', 0xA6: 'Œ', 0xA7: 'ʹ', 0xA8: '·',
	0xA9: '♭', 0xAA: '®', 0xAB: '±', 0xAC: 'Ơ', 0xAD: 'Ư', 0xAE: 'ʼ', 0xB0: 'ʻ', 0xB1: 'ł',
	0xB2: 'ø', 0xB3: 'đ', 0xB4: 'þ', 0xB5: 'æ', 0xB6: 'œ', 0xB7: 'ʺ', 0xB8: 'ı', 0xB9: '£',
	0xBA: 'ð', 0xBC: 'ơ', 0xBD: 'ư', 0xBE: '□', 0xBF: '■', 0xC0: '°', 0xC1: 'ℓ', 0xC2: '℗',
	0xC3: '©', 0xC4: '♯', 0xC5: '¿', 0xC6: '¡', 0xC7: 'ß', 0xC8: '€', 0xCD: 'e', 0xCE: 'o',
	0xCF: 'ß',
}

// anselCombining maps ANSEL's combining diacritics to Unicode combining
// marks. ANSEL writes them before the letter they modify, Unicode after.
var anselCombining = map[byte]rune{
	0xE0: '̉', 0xE1: '̀', 0xE2: '́', 0xE3: '̂', 0xE4: '̃', 0xE5: '̄',
	0xE6: '̆', 0xE7: '̇', 0xE8: '̈', 0xE9: '̌', 0xEA: '̊', 0xEB: '︠',
	0xEC: '︡', 0xED: '̕', 0xEE: '̋', 0xEF: '̐', 0xF0: '̧', 0xF1: '̨',
	0xF2: '̣', 0xF3: '̤', 0xF4: '̥', 0xF5: '̳', 0xF6: '̲', 0xF7: '̦',
	0xF8: '̜', 0xF9: '̮', 0xFA: '︢', 0xFB: '︣', 0xFE: '̓',
}

// decodeANSEL converts ANSEL bytes to a UTF-8 string in NFC, so "u" with
// a combining diaeresis becomes the single character "ü".
func decodeANSEL(data []byte, doc *Document) string {
	var b strings.Builder
	b.Grow(len(data))
	var marks []rune
	unknown := 0
	for _, c := range data {
		if mark, ok := anselCombining[c]; ok {
			marks = append(marks, mark)
			continue
		}
		var r rune
		switch {
		case c < 0x80:
			r = rune(c)
		default:
			var ok bool
			if r, ok = anselChars[c]; !ok {
				r = '�'
				unknown++
			}
		}
		b.WriteRune(r)
		// Marks belong to the character that follows them; a line break
		// is no character, so dangling marks there are dropped.
		if r != '\n' && r != '\r' {
			for _, m := range marks {
				b.WriteRune(m)
			}
		}
		marks = marks[:0]
	}
	if unknown > 0 {
		doc.warn(0, WarnANSEL, map[string]any{"count": unknown}, "%d ANSEL characters could not be read and were replaced with �", unknown)
	}
	return norm.NFC.String(b.String())
}
