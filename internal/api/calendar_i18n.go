package api

import (
	"fmt"
	"strings"
)

// calendarTexts are the words a calendar feed is written in. Names are never
// declined, so every phrase takes the name as a plain argument.
type calendarTexts struct {
	birthday    string // names, year
	anniversary string // names, year
	remembrance string // names, year
	from        string // tree name
}

var calendarLanguages = map[string]calendarTexts{
	"en": {
		birthday:    "Birthday: %s (born %d)",
		anniversary: "Wedding anniversary: %s (%d)",
		remembrance: "Remembrance: %s (died %d)",
		from:        "From the family tree “%s” in GoTree.",
	},
	"ru": {
		birthday:    "День рождения: %s (год рождения %d)",
		anniversary: "Годовщина свадьбы: %s (%d)",
		remembrance: "День памяти: %s (год смерти %d)",
		from:        "Из семейного древа «%s» в GoTree.",
	},
	"kk": {
		birthday:    "Туған күн: %s (туған жылы %d)",
		anniversary: "Үйлену тойының жылдығы: %s (%d)",
		remembrance: "Еске алу күні: %s (қайтыс болған жылы %d)",
		from:        `GoTree-дегі "%s" шежіресінен.`,
	},
}

// calendarTextsFor picks the texts for the lang query value. Anything missing
// or unknown gives English, so existing subscriptions keep their wording.
func calendarTextsFor(lang string) calendarTexts {
	lang, _, _ = strings.Cut(strings.ToLower(strings.TrimSpace(lang)), "-")
	if t, ok := calendarLanguages[lang]; ok {
		return t
	}
	return calendarLanguages["en"]
}

func (t calendarTexts) summary(kind, names string, year int) string {
	switch kind {
	case "BIRT":
		return fmt.Sprintf(t.birthday, names, year)
	case "MARR":
		return fmt.Sprintf(t.anniversary, names, year)
	default:
		return fmt.Sprintf(t.remembrance, names, year)
	}
}
