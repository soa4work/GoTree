package match

import "testing"

func TestCyrillicPhonetic(t *testing.T) {
	same := [][]string{
		{"Мейер", "Meyer", "Майер", "Mayer"},
		{"Иванов", "Ivanov", "Iwanow", "иванов"},
		{"Шмидт", "Schmidt", "Шмитт"},
		{"Қасымов", "Касымов", "Kasymov"},
		{"Әлиев", "Алиев", "Aliev"},
	}
	for _, group := range same {
		want := Phonetic(Normalize(group[0]))
		for _, name := range group[1:] {
			if got := Phonetic(Normalize(name)); got != want {
				t.Errorf("%s: %q, want %q like %s", name, got, want, group[0])
			}
		}
	}
	if a, b := Phonetic(Normalize("Иванов")), Phonetic(Normalize("Петров")); a == b {
		t.Errorf("Иванов and Петров code alike: %q", a)
	}
}

func TestNormalizeCyrillic(t *testing.T) {
	for in, want := range map[string]string{
		"Щербаков":  "shcherbakov",
		"Жуков":     "zhukov",
		"Ёлкин":     "elkin",
		"Новосёлов": "novoselov",
		"Юрьев":     "yurev",
		"Хасанов":   "khasanov",
		"Цой":       "tsoi",
		"Ғабит":     "gabit",
		"Нұрғали":   "nurgali",
		"Өтеп-Һаж":  "otephazh",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}

func TestScoreAcrossScripts(t *testing.T) {
	q := Query{Given: "Иван", Surname: "Мейер", Sex: "M", BirthYear: 1850}
	for _, c := range []Candidate{
		{Given: "Iwan", Surname: "Meyer", Sex: "M", BirthYear: 1850},
		{Given: "Иван", Surname: "Майер", Sex: "M", BirthYear: 1851},
	} {
		if s := Score(q, c); s < 0.85 {
			t.Errorf("%s %s: %.2f", c.Given, c.Surname, s)
		}
	}
	if s := Score(q, Candidate{Given: "Пётр", Surname: "Петров", Sex: "M", BirthYear: 1850}); s > 0.7 {
		t.Errorf("other person scores %.2f", s)
	}
}
