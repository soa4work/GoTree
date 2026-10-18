package store

import (
	"reflect"
	"testing"
)

func TestPersonsSortCyrillicAndKazakh(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	for _, surname := range []string{
		"яковлев", "Яков", "Zimmer", "Үлкен", "Уваров", "Абай", "Әлиев", "Алиев", "Ғабитов", "Гоголь",
		"Қасымов", "Кузнецов", "Нұров", "Ңұров", "Орлов", "Өтегенов", "Хан", "Һашимов", "Müller", "muller", "Meyer",
		"Иванов", "Іле", "иванов", "Елкин", "Ермаков",
	} {
		mustPerson(t, s, a, "Имя", surname)
	}
	list, err := s.ListPersons(ctx, a, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range list.Items {
		got = append(got, p.Surname)
	}
	// Latin first, then Cyrillic; Kazakh letters follow their base letters
	// and case does not split a letter in two.
	want := []string{
		"Meyer", "Müller", "muller", "Zimmer",
		"Абай", "Алиев", "Әлиев", "Гоголь", "Ғабитов", "Елкин", "Ермаков",
		"Иванов", "иванов", "Іле", "Кузнецов", "Қасымов", "Нұров", "Ңұров", "Орлов", "Өтегенов",
		"Уваров", "Үлкен", "Хан", "Һашимов", "Яков", "яковлев",
	}
	// Names equal ignoring case and accents keep their insertion order.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order:\n got %v\nwant %v", got, want)
	}
}

func TestPersonSearchCyrillic(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	elkin := mustPerson(t, s, a, "Пётр", "Ёлкин")
	mustPerson(t, s, a, "Иван", "Петров")
	for _, q := range []string{"елкин", "ЁЛКИН", "Ёлкин", "ел", "петр", "Пётр Елкин"} {
		list, err := s.ListPersons(ctx, a, q, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range list.Items {
			found = found || p.ID == elkin.ID
		}
		if !found {
			t.Errorf("search %q does not find Пётр Ёлкин", q)
		}
	}
	list, err := s.ListPersons(ctx, a, "иван", 10, 0)
	if err != nil || len(list.Items) != 1 || list.Items[0].GivenNames != "Иван" {
		t.Errorf("search иван: %+v %v", list, err)
	}
}

func TestPlacesAndSourcesSortCyrillic(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	for _, n := range []string{"Шымкент", "алматы", "Астана", "Әулиеата", "Zürich", "Ақтау"} {
		if _, err := s.CreatePlace(ctx, a, PlaceInput{Name: n}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateSource(ctx, a, SourceInput{Title: n}); err != nil {
			t.Fatal(err)
		}
	}
	places, err := s.ListPlaces(ctx, a, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	var gotPlaces []string
	for _, p := range places {
		gotPlaces = append(gotPlaces, p.Name)
	}
	// "Әулиеата" sorts right after "а…" names, not after "я".
	want := []string{"Zürich", "Ақтау", "алматы", "Астана", "Әулиеата", "Шымкент"}
	if !reflect.DeepEqual(gotPlaces, want) {
		t.Errorf("places: %v, want %v", gotPlaces, want)
	}
	sources, err := s.ListSources(ctx, a, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	var gotSources []string
	for _, p := range sources {
		gotSources = append(gotSources, p.Title)
	}
	if !reflect.DeepEqual(gotSources, want) {
		t.Errorf("sources: %v, want %v", gotSources, want)
	}
}

func TestPersonsSortByCyrillicGivenNames(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	for _, given := range []string{"Яков", "иван", "Әлия", "Иван", "Алия", "Нұрлан"} {
		mustPerson(t, s, a, given, "Ахметов")
	}
	list, err := s.ListPersons(ctx, a, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range list.Items {
		got = append(got, p.GivenNames)
	}
	want := []string{"Алия", "Әлия", "иван", "Иван", "Нұрлан", "Яков"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
