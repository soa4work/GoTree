package store

import (
	"reflect"
	"sort"
	"testing"
)

func TestPersonsSortCyrillicAndKazakh(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	for _, surname := range []string{
		"яковлев", "Яков", "Zimmer", "Үлкен", "Уваров", "Абай", "Әлиев", "Алиев", "Ғабитов", "Гоголь",
		"Қасымов", "Кузнецов", "Йодин", "Ұлан", "Ыбырай", "Нұров", "Ңұров", "Орлов", "Өтегенов", "Хан", "Һашимов", "Müller", "muller", "Meyer",
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
	// Latin first, then Cyrillic in alphabet order (й after и, ұ before ү,
	// і after ы); case does not split a letter in two.
	want := []string{
		"Meyer", "Müller", "muller", "Zimmer",
		"Абай", "Алиев", "Әлиев", "Гоголь", "Ғабитов", "Елкин", "Ермаков",
		"Иванов", "иванов", "Йодин", "Кузнецов", "Қасымов", "Нұров", "Ңұров", "Орлов", "Өтегенов",
		"Уваров", "Ұлан", "Үлкен", "Хан", "Һашимов", "Ыбырай", "Іле", "Яков", "яковлев",
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

func placeNames(list []Place) []string {
	got := []string{}
	for _, p := range list {
		got = append(got, p.Name)
	}
	sort.Strings(got)
	return got
}

func TestListPlacesSearchIgnoresCyrillicCase(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	mk := func(name string, parent *int64) int64 {
		p, err := s.CreatePlace(ctx, a, PlaceInput{Name: name, ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	kz := mk("Казахстан", nil)
	for _, n := range []string{"Алматы", "Әулиеата", "Қарағанды", "Өскемен", "Үштөбе", "Іле"} {
		mk(n, &kz)
	}
	ru := mk("Россия", nil)
	mk("Москва", &ru)
	mk("Ёлкино", nil)
	mk("Berlin", nil)

	for _, c := range []struct {
		q    string
		want []string
	}{
		{"москва", []string{"Москва"}},
		{"МОСКВА", []string{"Москва"}},
		{"мОсКвА", []string{"Москва"}},
		{" москва ", []string{"Москва"}},
		{"россия", []string{"Москва", "Россия"}},
		{"әулие", []string{"Әулиеата"}},
		{"ӘУЛИЕ", []string{"Әулиеата"}},
		{"қараған", []string{"Қарағанды"}},
		{"ҚАРАҒАН", []string{"Қарағанды"}},
		{"өскемен", []string{"Өскемен"}},
		{"ҮШТӨБЕ", []string{"Үштөбе"}},
		{"іле", []string{"Іле"}},
		{"ІЛЕ", []string{"Іле"}},
		{"елкино", []string{"Ёлкино"}},
		{"ЁЛКИНО", []string{"Ёлкино"}},
		{"Ёлкино", []string{"Ёлкино"}},
		{"казахстан", []string{"Алматы", "Казахстан", "Қарағанды", "Іле", "Әулиеата", "Үштөбе", "Өскемен"}},
		{"berlin", []string{"Berlin"}},
		{"_", []string{}},
		{"%", []string{}},
		{"нет такого", []string{}},
	} {
		list, err := s.ListPlaces(ctx, a, c.q, 100)
		if err != nil {
			t.Fatal(err)
		}
		want := append([]string{}, c.want...)
		sort.Strings(want)
		if got := placeNames(list); !reflect.DeepEqual(got, want) {
			t.Errorf("ListPlaces(%q) = %v, want %v", c.q, got, want)
		}
	}
}

func TestListSourcesAndRepositoriesSearchIgnoresCyrillicCase(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	for _, n := range []string{"Центральный государственный архив", "Қазақстан ұлттық архиві"} {
		if _, err := s.CreateRepository(ctx, a, RepositoryInput{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	for _, in := range []SourceInput{
		{Title: "Метрическая книга Успенской церкви", Author: "Әбілхан Қасымов"},
		{Title: "Kirchenbuch", Author: "Müller"},
	} {
		if _, err := s.CreateSource(ctx, a, in); err != nil {
			t.Fatal(err)
		}
	}

	for _, c := range []struct {
		q    string
		want []string
	}{
		{"центральный", []string{"Центральный государственный архив"}},
		{"ЦЕНТРАЛЬНЫЙ", []string{"Центральный государственный архив"}},
		{"ҚАЗАҚСТАН", []string{"Қазақстан ұлттық архиві"}},
		{"ұлттық", []string{"Қазақстан ұлттық архиві"}},
		{"архив", []string{"Центральный государственный архив", "Қазақстан ұлттық архиві"}},
	} {
		list, err := s.ListRepositories(ctx, a, c.q)
		if err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for _, r := range list {
			got = append(got, r.Name)
		}
		sort.Strings(got)
		want := append([]string{}, c.want...)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ListRepositories(%q) = %v, want %v", c.q, got, want)
		}
	}

	for _, c := range []struct{ q, want string }{
		{"метрическая", "Метрическая книга Успенской церкви"},
		{"УСПЕНСКОЙ", "Метрическая книга Успенской церкви"},
		{"әбілхан", "Метрическая книга Успенской церкви"},
		{"ҚАСЫМОВ", "Метрическая книга Успенской церкви"},
		{"kirchen", "Kirchenbuch"},
		{"MÜLLER", "Kirchenbuch"},
	} {
		list, err := s.ListSources(ctx, a, c.q, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].Title != c.want {
			t.Errorf("ListSources(%q) = %+v, want [%s]", c.q, list, c.want)
		}
	}
}

func TestListHeirloomsSearchIgnoresCyrillicCase(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	for _, in := range []HeirloomInput{
		{Name: "Серебряные часы", Description: "Подарок от Ақмарал"},
		{Name: "Pocket watch"},
	} {
		if _, err := s.CreateHeirloom(ctx, a, in); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ q, want string }{
		{"серебряные", "Серебряные часы"},
		{"ЧАСЫ", "Серебряные часы"},
		{"ақмарал", "Серебряные часы"},
		{"АҚМАРАЛ", "Серебряные часы"},
		{"WATCH", "Pocket watch"},
	} {
		list, err := s.ListHeirlooms(ctx, a, c.q, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].Name != c.want {
			t.Errorf("ListHeirlooms(%q) = %+v, want [%s]", c.q, list, c.want)
		}
	}
}
