package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praetorianer777/gotree/internal/db"
)

var ctx = context.Background()

func newStore(t *testing.T) *Store {
	t.Helper()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	s := New(conn)
	s.Now = func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) }
	return s
}

// setup creates the admin and returns an actor in their tree.
func setup(t *testing.T, s *Store) Actor {
	t.Helper()
	u, err := s.Setup(ctx, SetupInput{Username: "admin", Password: "correct horse battery"})
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.CreateSession(ctx, u.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.SessionByToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	return sess.Actor()
}

// otherTree adds a second tree with its own owner, sharing the database.
func otherTree(t *testing.T, s *Store) Actor {
	t.Helper()
	res, err := s.DB.Exec(`INSERT INTO users (username, password_hash, created_at, updated_at) VALUES ('other', 'x', '', '')`)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	res, err = s.DB.Exec(`INSERT INTO trees (name, created_at, updated_at) VALUES ('Other', '', '')`)
	if err != nil {
		t.Fatal(err)
	}
	tid, _ := res.LastInsertId()
	if _, err := s.DB.Exec(`INSERT INTO tree_members VALUES (?, ?, 'owner')`, tid, uid); err != nil {
		t.Fatal(err)
	}
	return Actor{UserID: uid, TreeID: tid, Role: RoleOwner}
}

func mustPerson(t *testing.T, s *Store, a Actor, given, surname string) Person {
	t.Helper()
	p, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: given, Surname: surname})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func ptr[T any](v T) *T { return &v }

func validationField(t *testing.T, err error, field string) {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got %v, want a validation error on %s", err, field)
	}
	if _, ok := ve.Fields[field]; !ok {
		t.Fatalf("validation error %v does not mention %s", ve, field)
	}
}

func TestSetupAndAuthentication(t *testing.T) {
	s := newStore(t)
	need, err := s.NeedsSetup(ctx)
	if err != nil || !need {
		t.Fatalf("NeedsSetup = %v, %v; want true", need, err)
	}

	_, err = s.Setup(ctx, SetupInput{Username: "admin", Password: "short"})
	validationField(t, err, "password")
	_, err = s.Setup(ctx, SetupInput{Username: "a b", Password: "long enough password"})
	validationField(t, err, "username")

	u, err := s.Setup(ctx, SetupInput{Username: "Admin", Password: "correct horse battery", TreeName: "Müller family"})
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != "admin" {
		t.Errorf("role %q, want admin", u.Role)
	}
	if _, err := s.Setup(ctx, SetupInput{Username: "second", Password: "correct horse battery"}); !errors.Is(err, ErrSetupDone) {
		t.Errorf("second setup: got %v, want ErrSetupDone", err)
	}

	if _, err := s.Authenticate(ctx, "admin", "wrong password!"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("wrong password: got %v", err)
	}
	if _, err := s.Authenticate(ctx, "nobody", "correct horse battery"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("unknown user: got %v", err)
	}
	// Usernames are case-insensitive.
	got, err := s.Authenticate(ctx, "ADMIN", "correct horse battery")
	if err != nil || got.ID != u.ID {
		t.Fatalf("Authenticate = %+v, %v", got, err)
	}

	token, err := s.CreateSession(ctx, u.ID, "browser")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.SessionByToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if sess.TreeID == 0 || sess.TreeRole != RoleOwner {
		t.Errorf("session tree %d role %q, want the owned tree", sess.TreeID, sess.TreeRole)
	}
	if name, _ := s.TreeName(ctx, sess.TreeID); name != "Müller family" {
		t.Errorf("tree name %q", name)
	}

	// Sessions expire after the TTL.
	s.Now = func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC).Add(SessionTTL + time.Second) }
	if _, err := s.SessionByToken(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session: got %v, want ErrNotFound", err)
	}

	if err := s.DeleteSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByToken(ctx, "not-a-token"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown token: got %v", err)
	}
}

func TestPersonsAndSearch(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)

	p, err := s.CreatePerson(ctx, a, PersonInput{
		GivenNames: "  Anna   Maria ", Surname: "Müller", Sex: "f",
		AlternateNames: []AlternateNameInput{{Type: "married", Surname: "Schneider"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.GivenNames != "Anna Maria" || p.Sex != "F" || len(p.AlternateNames) != 1 {
		t.Fatalf("person not normalized: %+v", p)
	}
	mustPerson(t, s, a, "Karl", "Schmidt")
	mustPerson(t, s, a, "Anton", "")

	_, err = s.CreatePerson(ctx, a, PersonInput{Sex: "Q"})
	validationField(t, err, "sex")
	_, err = s.CreatePerson(ctx, a, PersonInput{AlternateNames: []AlternateNameInput{{Type: "married"}}})
	validationField(t, err, "alternateNames")

	search := func(q string) []int64 {
		t.Helper()
		list, err := s.ListPersons(ctx, a, q, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, r := range list.Items {
			ids = append(ids, r.ID)
		}
		return ids
	}
	for _, q := range []string{"muller", "MÜLL", "anna mül", "schneider", "mar"} {
		if ids := search(q); len(ids) != 1 || ids[0] != p.ID {
			t.Errorf("search %q = %v, want [%d]", q, ids, p.ID)
		}
	}
	if ids := search(`"; DROP TABLE persons; --`); len(ids) != 0 {
		t.Errorf("hostile query matched %v", ids)
	}

	all, err := s.ListPersons(ctx, a, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 3 || all.Items[0].Surname != "Müller" || all.Items[2].GivenNames != "Anton" {
		t.Errorf("alphabetical listing (people without surname last): %+v", all.Items)
	}

	// Renaming updates the search index.
	in := PersonInput{GivenNames: "Anna", Surname: "Weber"}
	if _, err := s.UpdatePerson(ctx, a, p.ID, in); err != nil {
		t.Fatal(err)
	}
	if ids := search("schneider"); len(ids) != 0 {
		t.Errorf("old alternate name still found: %v", ids)
	}
	if ids := search("weber"); len(ids) != 1 {
		t.Errorf("new name not found: %v", ids)
	}

	if err := s.DeletePerson(ctx, a, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPerson(ctx, a, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted person: got %v", err)
	}
	if ids := search("weber"); len(ids) != 0 {
		t.Errorf("deleted person still searchable: %v", ids)
	}
}

func TestLivingInference(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)

	addEvent := func(p Person, typ, date string) {
		t.Helper()
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: typ, Date: date}); err != nil {
			t.Fatal(err)
		}
	}
	unknown := mustPerson(t, s, a, "Nobody", "Known")
	young := mustPerson(t, s, a, "Young", "One")
	addEvent(young, "BIRT", "1990")
	old := mustPerson(t, s, a, "Old", "One")
	addEvent(old, "BIRT", "1850")
	dead := mustPerson(t, s, a, "Dead", "One")
	addEvent(dead, "DEAT", "")
	stated, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Stated", IsLiving: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		p    Person
		want bool
	}{{unknown, true}, {young, true}, {old, false}, {dead, false}, {stated, false}} {
		got, err := s.GetPerson(ctx, a, tc.p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Living != tc.want {
			t.Errorf("%s: living = %v, want %v", tc.p.GivenNames, got.Living, tc.want)
		}
	}
}

func TestPlaces(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)

	country, err := s.CreatePlace(ctx, a, PlaceInput{Name: "Germany", PlaceType: "Country"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.CreatePlace(ctx, a, PlaceInput{Name: "Brandenburg", ParentID: &country.ID})
	if err != nil {
		t.Fatal(err)
	}
	city, err := s.CreatePlace(ctx, a, PlaceInput{Name: "Potsdam", ParentID: &state.ID, Lat: ptr(52.4), Lng: ptr(13.06)})
	if err != nil {
		t.Fatal(err)
	}
	if city.FullName != "Potsdam, Brandenburg, Germany" {
		t.Errorf("full name %q", city.FullName)
	}
	if country.PlaceType != "country" {
		t.Errorf("place type not normalized: %q", country.PlaceType)
	}

	_, err = s.CreatePlace(ctx, a, PlaceInput{Name: "Berlin, Germany"})
	validationField(t, err, "name")
	_, err = s.CreatePlace(ctx, a, PlaceInput{Name: "X", Lat: ptr(1.0)})
	validationField(t, err, "lat")
	_, err = s.UpdatePlace(ctx, a, country.ID, PlaceInput{Name: "Germany", ParentID: &city.ID})
	validationField(t, err, "parentId")

	found, err := s.ListPlaces(ctx, a, "brandenburg", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Errorf("filter on a parent's name should find the state and the city, got %d", len(found))
	}

	if err := s.DeletePlace(ctx, a, state.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("deleting a place with children: got %v, want ErrConflict", err)
	}
	p := mustPerson(t, s, a, "Friedrich", "")
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "BIRT", PlaceID: &city.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePlace(ctx, a, city.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("deleting a used place: got %v, want ErrConflict", err)
	}
}

func TestEventsDatesAndParticipants(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	head := mustPerson(t, s, a, "Johann", "Weber")
	wife := mustPerson(t, s, a, "Maria", "Weber")

	census, err := s.CreateEvent(ctx, a, EventInput{
		PersonID: &head.ID, Type: "cens", Date: "1900-06-01",
		Participants: []ParticipantInput{{PersonID: wife.ID, Role: "Spouse"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if census.Type != "CENS" || census.Date.Normalized != "1 JUN 1900" || !census.Date.Valid || *census.Date.SortKey != 19000601 {
		t.Errorf("event not normalized: %+v", census)
	}
	if census.Date.Raw != "1900-06-01" {
		t.Errorf("raw date must be kept as entered, got %q", census.Date.Raw)
	}
	if len(census.Participants) != 1 || census.Participants[0].Person.ID != wife.ID || census.Participants[0].Role != "spouse" {
		t.Errorf("participants: %+v", census.Participants)
	}

	odd, err := s.CreateEvent(ctx, a, EventInput{PersonID: &head.ID, Type: "OCCU", Date: "sometime in spring", Description: "Weaver"})
	if err != nil {
		t.Fatal(err)
	}
	if odd.Date.Valid || odd.Date.Raw != "sometime in spring" || odd.Date.SortKey != nil {
		t.Errorf("unparseable date must be kept, not normalized: %+v", odd.Date)
	}

	// The wife sees the shared census with her role.
	detail, err := s.GetPersonDetail(ctx, a, wife.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Events) != 1 || detail.Events[0].ID != census.ID || detail.Events[0].Role != "spouse" {
		t.Errorf("shared event on participant: %+v", detail.Events)
	}

	cases := []struct {
		name  string
		in    EventInput
		field string
	}{
		{"no owner", EventInput{Type: "BIRT"}, "personId"},
		{"two owners", EventInput{PersonID: &head.ID, FamilyID: ptr(int64(1)), Type: "BIRT"}, "personId"},
		{"bad type", EventInput{PersonID: &head.ID, Type: "birth day"}, "type"},
		{"custom without label", EventInput{PersonID: &head.ID, Type: "EVEN"}, "customLabel"},
		{"marriage on a person", EventInput{PersonID: &head.ID, Type: "MARR"}, "type"},
		{"disputed without reason", EventInput{PersonID: &head.ID, Type: "BIRT", Status: "disputed"}, "statusReason"},
		{"unknown role", EventInput{PersonID: &head.ID, Type: "CENS", Participants: []ParticipantInput{{PersonID: wife.ID, Role: "boss"}}}, "participants"},
		{"principal as participant", EventInput{PersonID: &head.ID, Type: "CENS", Participants: []ParticipantInput{{PersonID: head.ID, Role: "head"}}}, "participants"},
		{"missing place", EventInput{PersonID: &head.ID, Type: "BIRT", PlaceID: ptr(int64(999))}, "placeId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.CreateEvent(ctx, a, tc.in)
			validationField(t, err, tc.field)
		})
	}

	disproven, err := s.UpdateEvent(ctx, a, odd.ID, EventInput{
		PersonID: &head.ID, Type: "OCCU", Description: "Weaver", Status: "disproven", StatusReason: "Confused with his cousin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if disproven.Status != StatusDisproven {
		t.Errorf("status %q", disproven.Status)
	}
}

func TestFamilies(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	father := mustPerson(t, s, a, "Hans", "Weber")
	child := mustPerson(t, s, a, "Paul", "Weber")
	grandchild := mustPerson(t, s, a, "Lena", "Weber")

	// The mother is unknown: no placeholder person is created.
	fam, err := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &father.ID, UnionType: "married"})
	if err != nil {
		t.Fatal(err)
	}
	if fam.Partner2 != nil {
		t.Error("unknown partner must be nil")
	}
	fam, err = s.SetChild(ctx, a, fam.ID, child.ID, ChildInput{RelationPartner1: "adopted"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fam.Children) != 1 || fam.Children[0].RelationPartner1 != "adopted" || fam.Children[0].RelationPartner2 != "birth" {
		t.Errorf("children: %+v", fam.Children)
	}

	childFam, err := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &child.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetChild(ctx, a, childFam.ID, grandchild.ID, ChildInput{}); err != nil {
		t.Fatal(err)
	}

	// Cycles: the grandfather cannot become his grandson's child, and the
	// grandchild cannot become a partner of the family she descends from.
	_, err = s.SetChild(ctx, a, childFam.ID, father.ID, ChildInput{})
	validationField(t, err, "childId")
	_, err = s.SetChild(ctx, a, childFam.ID, child.ID, ChildInput{})
	validationField(t, err, "childId")
	_, err = s.UpdateFamily(ctx, a, fam.ID, FamilyInput{Partner1ID: &father.ID, Partner2ID: &grandchild.ID})
	validationField(t, err, "partner2Id")
	_, err = s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &father.ID, Partner2ID: &father.ID})
	validationField(t, err, "partner2Id")

	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &fam.ID, Type: "MARR", Date: "1920"}); err != nil {
		t.Fatal(err)
	}

	detail, err := s.GetPersonDetail(ctx, a, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ParentFamilies) != 1 || detail.ParentFamilies[0].ID != fam.ID {
		t.Errorf("parent families: %+v", detail.ParentFamilies)
	}
	if len(detail.ParentFamilies[0].Events) != 1 {
		t.Errorf("family events missing: %+v", detail.ParentFamilies[0])
	}
	if len(detail.PartnerFamilies) != 1 || len(detail.PartnerFamilies[0].Children) != 1 {
		t.Errorf("partner families: %+v", detail.PartnerFamilies)
	}

	if _, err := s.RemoveChild(ctx, a, fam.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveChild(ctx, a, fam.ID, child.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing a child twice: got %v", err)
	}

	// Deleting the only partner of a family without children or events
	// removes the empty family.
	if err := s.DeletePerson(ctx, a, grandchild.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePerson(ctx, a, child.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFamily(ctx, a, childFam.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty family should be gone, got %v", err)
	}
	// The family with a marriage event stays even after the partner goes.
	if err := s.DeletePerson(ctx, a, father.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFamily(ctx, a, fam.ID); err != nil {
		t.Errorf("family with events should stay: %v", err)
	}
}

func TestTreeIsolation(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	b := otherTree(t, s)

	mine := mustPerson(t, s, a, "Mine", "")
	place, err := s.CreatePlace(ctx, a, PlaceInput{Name: "Here"})
	if err != nil {
		t.Fatal(err)
	}
	theirs := mustPerson(t, s, b, "Theirs", "")

	if _, err := s.GetPerson(ctx, b, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("read across trees: got %v", err)
	}
	if _, err := s.UpdatePerson(ctx, b, mine.ID, PersonInput{GivenNames: "Hijacked"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update across trees: got %v", err)
	}
	if err := s.DeletePerson(ctx, b, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete across trees: got %v", err)
	}
	_, err = s.CreateEvent(ctx, b, EventInput{PersonID: &theirs.ID, Type: "BIRT", PlaceID: &place.ID})
	validationField(t, err, "placeId")
	_, err = s.CreateFamily(ctx, b, FamilyInput{Partner1ID: &theirs.ID, Partner2ID: &mine.ID})
	validationField(t, err, "partner2Id")

	list, err := s.ListPersons(ctx, b, "mine", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 0 {
		t.Errorf("search leaked across trees: %+v", list)
	}
}

func TestChangeLog(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	p := mustPerson(t, s, a, "Logged", "")
	if _, err := s.UpdatePerson(ctx, a, p.ID, PersonInput{GivenNames: "Renamed"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePerson(ctx, a, p.ID); err != nil {
		t.Fatal(err)
	}

	rows, err := s.DB.Query(`
		SELECT action, before_json IS NOT NULL, after_json IS NOT NULL, user_id
		FROM change_log WHERE entity_type = 'person' AND entity_id = ? ORDER BY id`, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type entry struct {
		action        string
		before, after bool
		user          int64
	}
	var got []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.action, &e.before, &e.after, &e.user); err != nil {
			t.Fatal(err)
		}
		got = append(got, e)
	}
	want := []entry{{"create", false, true, a.UserID}, {"update", true, true, a.UserID}, {"delete", true, false, a.UserID}}
	if len(got) != len(want) {
		t.Fatalf("change log %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPersonNameNotes(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	p, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Anna", Surname: "Weber", NameNotes: "  spelled Maier ",
		AlternateNames: []AlternateNameInput{{Type: "married", GivenNames: "Anna", Surname: "Schmidt", Notes: "after 1890"}}})
	if err != nil {
		t.Fatal(err)
	}
	if p.NameNotes != "spelled Maier" || p.AlternateNames[0].Notes != "after 1890" {
		t.Fatalf("created: %q %+v", p.NameNotes, p.AlternateNames)
	}
	id := p.AlternateNames[0].ID
	p, err = s.UpdatePerson(ctx, a, p.ID, PersonInput{GivenNames: "Anna", Surname: "Weber", NameNotes: "changed",
		AlternateNames: []AlternateNameInput{{ID: &id, Type: "married", GivenNames: "Anna", Surname: "Schmidt", Notes: "later"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPersonDetail(ctx, a, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NameNotes != "changed" || got.AlternateNames[0].Notes != "later" {
		t.Errorf("updated: %q %+v", got.NameNotes, got.AlternateNames)
	}
	long := strings.Repeat("x", 100_001)
	_, err = s.CreatePerson(ctx, a, PersonInput{GivenNames: "A", NameNotes: long})
	validationField(t, err, "nameNotes")
	_, err = s.CreatePerson(ctx, a, PersonInput{GivenNames: "A", AlternateNames: []AlternateNameInput{{Type: "other", GivenNames: "B", Notes: long}}})
	validationField(t, err, "alternateNames")
}
