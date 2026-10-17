package store

import (
	"strings"
	"testing"
)

func TestHeirlooms(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	anna := mustPerson(t, s, a, "Anna", "Weber")
	paul := mustPerson(t, s, a, "Paul", "Weber")
	src := mustSource(t, s, a, "Testament Anna Weber")

	_, err := s.CreateHeirloom(ctx, a, HeirloomInput{Name: " ", Kind: "spaceship"})
	validationField(t, err, "name")
	validationField(t, err, "kind")
	_, err = s.CreateHeirloom(ctx, a, HeirloomInput{Name: "x", Custody: []CustodyInput{{}}})
	validationField(t, err, "custody")

	h, err := s.CreateHeirloom(ctx, a, HeirloomInput{
		Name: "Pocket watch", Kind: "jewellery", Description: "Silver, engraved J.W.", MadeDate: "abt 1880",
		CurrentLocation: "Paul's desk",
		Custody: []CustodyInput{
			{PersonID: &anna.ID, FromDate: "1920", ToDate: "1965", How: "inherited"},
			{PersonID: &paul.ID, FromDate: "1965", How: "inherited", Notes: "left to him in her will"},
		},
		AddCitations: []NewCitation{{SourceID: src.ID, Page: "§ 3"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if h.MadeDate != "abt 1880" || len(h.Custody) != 2 || h.Custody[1].Person.GivenNames != "Paul" || len(h.Citations) != 1 {
		t.Fatalf("heirloom: %+v", h)
	}

	list, err := s.ListHeirlooms(ctx, a, "watch", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Holder == nil || list[0].Holder.ID != paul.ID {
		t.Errorf("list: %+v", list)
	}
	held, _ := s.ListHeirlooms(ctx, a, "", anna.ID)
	if len(held) != 1 {
		t.Errorf("held by Anna: %+v", held)
	}

	// Deleting a holder keeps the custody entry, without the person.
	if err := s.DeletePerson(ctx, a, anna.ID); err != nil {
		t.Fatal(err)
	}
	h, _ = s.GetHeirloom(ctx, a, h.ID)
	if len(h.Custody) != 2 || h.Custody[0].Person != nil || h.Custody[0].FromDate != "1920" {
		t.Errorf("custody after deleting the holder: %+v", h.Custody)
	}

	h, err = s.UpdateHeirloom(ctx, a, h.ID, HeirloomInput{Name: "Pocket watch", Kind: "jewellery", RemoveCitations: []int64{h.Citations[0].CitationID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Custody) != 0 || len(h.Citations) != 0 {
		t.Errorf("updated: %+v", h)
	}
	if err := s.DeleteHeirloom(ctx, a, h.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListHeirlooms(ctx, a, "", 0); len(list) != 0 {
		t.Errorf("deleted: %+v", list)
	}
}

func TestHeirloomGEDCOMRoundTrip(t *testing.T) {
	for _, v7 := range []bool{false, true} {
		s := newStore(t)
		a := setup(t, s)
		anna := mustPerson(t, s, a, "Anna", "Weber")
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &anna.ID, Type: "DEAT", Date: "1965"}); err != nil {
			t.Fatal(err)
		}
		src := mustSource(t, s, a, "Testament")
		place, err := s.CreatePlace(ctx, a, PlaceInput{Name: "Leipzig"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateHeirloom(ctx, a, HeirloomInput{
			Name: "Chest", Kind: "furniture", Description: "Oak", MadeDate: "1850", OriginPlaceID: &place.ID, CurrentLocation: "Attic",
			Notes: "Painted 1900",
			Custody: []CustodyInput{
				{PersonID: &anna.ID, FromDate: "1900", ToDate: "1965", How: "made"},
				{FromDate: "1965", How: "purchased", Notes: "a dealer in Halle"},
			},
			AddCitations: []NewCitation{{SourceID: src.ID, Page: "f. 2"}},
		}); err != nil {
			t.Fatal(err)
		}

		text := exportText(t, s, a, ExportOptions{Version7: v7})
		for _, want := range []string{"0 @H1@ _HEIRLOOM", "1 NAME Chest", "1 TYPE furniture", "1 _CUST @I", "2 TYPE purchased", "1 _LOC Attic"} {
			if !strings.Contains(text, want) {
				t.Errorf("v7=%v: export lacks %q:\n%s", v7, want, text)
			}
		}
		if v7 && !strings.Contains(text, "2 TAG _HEIRLOOM "+extensionDocs+"_heirloom") {
			t.Error("GEDCOM 7 declares the extension")
		}

		s2, a2 := reimport(t, text)
		list, err := s2.ListHeirlooms(ctx, a2, "", 0)
		if err != nil || len(list) != 1 {
			t.Fatalf("v7=%v: reimported: %+v %v", v7, list, err)
		}
		h, _ := s2.GetHeirloom(ctx, a2, list[0].ID)
		if h.Kind != "furniture" || h.Description != "Oak" || h.MadeDate != "1850" || h.OriginPlace == nil || h.OriginPlace.FullName != "Leipzig" ||
			h.CurrentLocation != "Attic" || h.Notes != "Painted 1900" || len(h.Citations) != 1 || h.Citations[0].Page != "f. 2" {
			t.Errorf("v7=%v: heirloom: %+v", v7, h)
		}
		if len(h.Custody) != 2 || h.Custody[0].Person == nil || h.Custody[0].Person.GivenNames != "Anna" || h.Custody[0].How != "made" ||
			h.Custody[1].Person != nil || h.Custody[1].Notes != "a dealer in Halle" || h.Custody[1].FromDate != "1965" {
			t.Errorf("v7=%v: custody: %+v", v7, h.Custody)
		}

		r, err := s.VerifyExport(ctx, a, v7, "test")
		if err != nil {
			t.Fatal(err)
		}
		if !r.OK {
			t.Errorf("v7=%v: verify: %+v %+v", v7, r.Rows, r.Dropped)
		}
	}
}

func TestHeirloomExportPrivacy(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	living := mustPerson(t, s, a, "Lisa", "Weber")
	if _, err := s.CreateHeirloom(ctx, a, HeirloomInput{Name: "Ring", Custody: []CustodyInput{{PersonID: &living.ID, FromDate: "2010"}}}); err != nil {
		t.Fatal(err)
	}
	text := exportText(t, s, a, ExportOptions{Privacy: PrivacyExcludeLiving})
	if strings.Contains(text, "_CUST @I") || !strings.Contains(text, "1 _CUST\n2 _FROM 2010") {
		t.Errorf("a living holder left out keeps the entry without a pointer:\n%s", text)
	}
}
