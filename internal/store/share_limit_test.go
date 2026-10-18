package store

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSharePersonsLimitAndTotal(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	dead := map[int64]bool{}
	for i := 0; i < 150; i++ {
		p := mustPerson(t, s, a, "N", fmt.Sprintf("S%03d", i))
		if i%8 == 7 {
			continue // living: 19 of 150
		}
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "DEAT", Date: "1900"}); err != nil {
			t.Fatal(err)
		}
		dead[p.ID] = true
	}
	sh := shareFor(t, s, a, ShareLinkInput{Label: "All", Scope: ShareScopeTree, Privacy: SharePrivacyDeceased})

	all, err := s.ListPersons(ctx, a, "", 500, 0)
	if err != nil {
		t.Fatal(err)
	}
	var want []int64
	for _, p := range all.Items {
		if dead[p.ID] {
			want = append(want, p.ID)
		}
	}
	list, err := s.SharePersons(ctx, sh, "")
	if err != nil {
		t.Fatal(err)
	}
	var got []int64
	for _, p := range list.Items {
		got = append(got, p.ID)
	}
	if list.Total != len(want) || len(got) != 100 || !reflect.DeepEqual(got, want[:100]) {
		t.Errorf("total %d (want %d), %d items", list.Total, len(want), len(got))
	}

	list, err = s.SharePersons(ctx, sh, "S00")
	if err != nil {
		t.Fatal(err)
	}
	// S000..S009 minus the living S007.
	if list.Total != 9 || len(list.Items) != 9 {
		t.Errorf("search: total %d, %d items", list.Total, len(list.Items))
	}
	for _, p := range list.Items {
		if !dead[p.ID] {
			t.Errorf("living person %s listed", p.Surname)
		}
	}

	info, err := s.ShareInfo(ctx, sh)
	if err != nil {
		t.Fatal(err)
	}
	if info.StartID == nil || *info.StartID != want[0] {
		t.Errorf("start: %v, want %d", info.StartID, want[0])
	}
}
