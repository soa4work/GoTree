package store

import (
	"errors"
	"reflect"
	"testing"
)

func TestCalendarFeedKeepsEmptyName(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	p, err := s.CreatePerson(ctx, a, PersonInput{Sex: "F"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "BIRT", Date: "3 JAN 1900"}); err != nil {
		t.Fatal(err)
	}
	feed, err := s.CreateCalendarFeed(ctx, a, CalendarFeedInput{Label: "All", IncludeLiving: true})
	if err != nil {
		t.Fatal(err)
	}
	cal, err := s.CalendarByToken(ctx, feed.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(cal.Entries) != 1 || !reflect.DeepEqual(cal.Entries[0].Names, []string{""}) {
		t.Errorf("entries: %+v", cal.Entries)
	}
}

func TestCalendarFeed(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	f := shareTree(t, s, a)
	hans, err := s.GetPersonDetail(ctx, a, f.hans.ID)
	if err != nil {
		t.Fatal(err)
	}
	famID := hans.PartnerFamilies[0].ID
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &famID, Type: "MARR", Date: "29 FEB 1904"}); err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateCalendarFeed(ctx, a, CalendarFeedInput{Label: " "})
	validationField(t, err, "label")

	deceased, err := s.CreateCalendarFeed(ctx, a, CalendarFeedInput{Label: "Remembrance"})
	if err != nil {
		t.Fatal(err)
	}
	cal, err := s.CalendarByToken(ctx, deceased.Token)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]CalendarEntry{}
	for _, e := range cal.Entries {
		kinds[e.Kind+" "+e.Names[0]] = e
	}
	// Hans: exact birthday; his death has only a year. Hans and Erika's
	// wedding. Lisa is living.
	if len(cal.Entries) != 2 || kinds["BIRT Hans Weber"].Day != 3 || kinds["MARR Hans Weber"].Month != 2 {
		t.Errorf("deceased only: %+v", cal.Entries)
	}
	if m := kinds["MARR Hans Weber"]; len(m.Names) != 2 || m.Names[1] != "Erika Weber" || m.Year != 1904 {
		t.Errorf("wedding: %+v", m)
	}

	all, err := s.CreateCalendarFeed(ctx, a, CalendarFeedInput{Label: "Birthdays", IncludeLiving: true})
	if err != nil {
		t.Fatal(err)
	}
	cal, err = s.CalendarByToken(ctx, all.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(cal.Entries) != 3 {
		t.Errorf("with living: %+v", cal.Entries)
	}

	feeds, err := s.ListCalendarFeeds(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 2 || feeds[0].Token != "" || feeds[1].LastUsedAt == nil {
		t.Errorf("feeds: %+v", feeds)
	}
	if err := s.RevokeCalendarFeed(ctx, a, all.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CalendarByToken(ctx, all.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoked: %v", err)
	}
	viewer := a
	viewer.Role = RoleViewer
	if _, err := s.ListCalendarFeeds(ctx, viewer); !errors.Is(err, ErrOwnerOnly) {
		t.Errorf("viewer: %v", err)
	}
}
