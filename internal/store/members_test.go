package store

import (
	"errors"
	"strings"
	"testing"
)

// treeFixture is a tree with an owner, an editor, a viewer and an outsider.
type treeFixture struct {
	tree                 int64
	owner, ed, view, out User
}

func newFixture(t *testing.T, s *Store) treeFixture {
	t.Helper()
	setup(t, s)
	a := admin(t, s)
	mk := func(name string) User {
		acc := mustAccount(t, s, a, name, "")
		return acc.User
	}
	f := treeFixture{owner: mk("owner1"), ed: mk("editor1"), view: mk("viewer1"), out: mk("outsider")}
	tr, err := s.CreateTree(ctx, f.owner.ID, "Fixture")
	if err != nil {
		t.Fatal(err)
	}
	f.tree = tr.ID
	for _, m := range []struct {
		u    User
		role string
	}{{f.ed, RoleEditor}, {f.view, RoleViewer}} {
		if _, err := s.AddMember(ctx, f.owner.ID, f.tree, MemberInput{Username: m.u.Username, Role: m.role}); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestCreateAndListTrees(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	a := admin(t, s)
	bob := mustAccount(t, s, a, "bob", "")

	list, err := s.ListTrees(ctx, bob.ID, 0)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("no trees yet: %v, %v", list, err)
	}
	t1, err := s.CreateTree(ctx, bob.ID, "  Zeta ")
	if err != nil || t1.Name != "Zeta" || t1.Role != RoleOwner {
		t.Fatalf("create: %+v, %v", t1, err)
	}
	t2, err := s.CreateTree(ctx, bob.ID, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	list, err = s.ListTrees(ctx, bob.ID, t1.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v, %v", list, err)
	}
	if list[0].ID != t2.ID || list[0].Current || list[1].ID != t1.ID || !list[1].Current {
		t.Errorf("order or current: %+v", list)
	}

	_, err = s.CreateTree(ctx, bob.ID, "  ")
	validationField(t, err, "name")
	_, err = s.CreateTree(ctx, bob.ID, strings.Repeat("x", 201))
	validationField(t, err, "name")
}

func TestMemberPermissions(t *testing.T) {
	s := newStore(t)
	f := newFixture(t, s)
	// A fifth account whom the owner may add, remove and re-role.
	a := admin(t, s)
	target := mustAccount(t, s, a, "target", "").User

	type op struct {
		name string
		run  func(by User) error
	}
	ops := []op{
		{"list", func(by User) error { _, err := s.ListMembers(ctx, by.ID, f.tree); return err }},
		{"add", func(by User) error {
			_, err := s.AddMember(ctx, by.ID, f.tree, MemberInput{Username: target.Username, Role: RoleViewer})
			return err
		}},
		{"role", func(by User) error {
			_, err := s.SetMemberRole(ctx, by.ID, f.tree, f.view.ID, RoleViewer)
			return err
		}},
		{"remove other", func(by User) error { return s.RemoveMember(ctx, by.ID, f.tree, target.ID) }},
		{"rename", func(by User) error { _, err := s.RenameTree(ctx, by.ID, f.tree, "Renamed"); return err }},
	}
	actors := []struct {
		name string
		u    User
		want error
	}{
		{"editor", f.ed, ErrOwnerOnly},
		{"viewer", f.view, ErrOwnerOnly},
		{"outsider", f.out, ErrNotFound},
		{"owner", f.owner, nil},
	}
	// The owner goes last, so the add before the remove finds the target in place.
	for _, o := range ops {
		for _, ac := range actors {
			if err := o.run(ac.u); !errors.Is(err, ac.want) && !(ac.want == nil && err == nil) {
				t.Errorf("%s as %s: got %v, want %v", o.name, ac.name, err, ac.want)
			}
		}
	}

	// Leaving: every member may, an outsider cannot.
	if err := s.RemoveMember(ctx, f.out.ID, f.tree, f.out.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("outsider leaving: %v", err)
	}
	if err := s.RemoveMember(ctx, f.view.ID, f.tree, f.view.ID); err != nil {
		t.Errorf("viewer leaving: %v", err)
	}
	if err := s.RemoveMember(ctx, f.ed.ID, f.tree, f.ed.ID); err != nil {
		t.Errorf("editor leaving: %v", err)
	}
}

func TestAddMember(t *testing.T) {
	s := newStore(t)
	f := newFixture(t, s)
	a := admin(t, s)
	mustAccount(t, s, a, "Carol", "")

	_, err := s.AddMember(ctx, f.owner.ID, f.tree, MemberInput{Username: "nobody", Role: RoleViewer})
	validationField(t, err, "username")
	var ve *ValidationError
	if errors.As(err, &ve) && ve.Fields["username"] != "an account with this username does not exist" {
		t.Errorf("message %q", ve.Fields["username"])
	}
	_, err = s.AddMember(ctx, f.owner.ID, f.tree, MemberInput{Username: "  ", Role: RoleViewer})
	validationField(t, err, "username")

	m, err := s.AddMember(ctx, f.owner.ID, f.tree, MemberInput{Username: "carol", Role: RoleEditor})
	if err != nil || m.Username != "Carol" || m.Role != RoleEditor {
		t.Fatalf("case-insensitive add: %+v, %v", m, err)
	}
	_, err = s.AddMember(ctx, f.owner.ID, f.tree, MemberInput{Username: "Carol", Role: RoleViewer})
	validationField(t, err, "username")
	if errors.As(err, &ve) && ve.Fields["username"] != "is already a member of this tree" {
		t.Errorf("message %q", ve.Fields["username"])
	}
	_, err = s.AddMember(ctx, f.owner.ID, f.tree, MemberInput{Username: "outsider", Role: "admin"})
	validationField(t, err, "role")

	list, err := s.ListMembers(ctx, f.owner.ID, f.tree)
	if err != nil || len(list) != 4 || list[0].Role != RoleOwner || list[1].Username != "Carol" {
		t.Errorf("list: %+v, %v", list, err)
	}
}

func TestLastOwner(t *testing.T) {
	s := newStore(t)
	f := newFixture(t, s)
	if _, err := s.SetMemberRole(ctx, f.owner.ID, f.tree, f.owner.ID, RoleEditor); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demoting the only owner: %v", err)
	}
	if err := s.RemoveMember(ctx, f.owner.ID, f.tree, f.owner.ID); !errors.Is(err, ErrLastOwner) {
		t.Errorf("owner leaving: %v", err)
	}
	// Setting the same role is not a demotion.
	if _, err := s.SetMemberRole(ctx, f.owner.ID, f.tree, f.owner.ID, RoleOwner); err != nil {
		t.Errorf("same role: %v", err)
	}
	if _, err := s.SetMemberRole(ctx, f.owner.ID, f.tree, f.ed.ID, RoleOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetMemberRole(ctx, f.ed.ID, f.tree, f.owner.ID, RoleViewer); err != nil {
		t.Fatalf("one owner demotes the other: %v", err)
	}
	if _, err := s.ListMembers(ctx, f.owner.ID, f.tree); !errors.Is(err, ErrOwnerOnly) {
		t.Errorf("demoted owner lists members: %v", err)
	}
	if err := s.RemoveMember(ctx, f.owner.ID, f.tree, f.owner.ID); err != nil {
		t.Errorf("a demoted owner may leave: %v", err)
	}
	if err := s.RemoveMember(ctx, f.ed.ID, f.tree, f.ed.ID); !errors.Is(err, ErrLastOwner) {
		t.Errorf("the remaining owner leaving: %v", err)
	}
}

func TestRemoveMemberClearsSessions(t *testing.T) {
	s := newStore(t)
	f := newFixture(t, s)
	other, err := s.CreateTree(ctx, f.ed.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := s.CreateSession(ctx, f.ed.ID, "x")
	if err != nil {
		t.Fatal(err)
	}
	// The editor's session sits in the fixture tree (oldest); a second one is elsewhere.
	if err := s.SwitchTree(ctx, tok, f.ed.ID, f.tree); err != nil {
		t.Fatal(err)
	}
	tok2, _ := s.CreateSession(ctx, f.ed.ID, "y")
	if err := s.SwitchTree(ctx, tok2, f.ed.ID, other.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveMember(ctx, f.owner.ID, f.tree, f.ed.ID); err != nil {
		t.Fatal(err)
	}
	sess, err := s.SessionByToken(ctx, tok)
	if err != nil || sess.TreeID != 0 || sess.TreeRole != "" {
		t.Errorf("removed session: %+v, %v", sess, err)
	}
	var n int
	_ = s.DB.QueryRow(`SELECT count(*) FROM sessions WHERE tree_id = ?`, f.tree).Scan(&n)
	if n != 0 {
		t.Errorf("%d sessions still in the tree", n)
	}
	// The session in another tree is untouched.
	if sess, err := s.SessionByToken(ctx, tok2); err != nil || sess.TreeID != other.ID {
		t.Errorf("other session: %+v, %v", sess, err)
	}
}

func TestRoleChangeAppliesImmediately(t *testing.T) {
	s := newStore(t)
	f := newFixture(t, s)
	tok, err := s.CreateSession(ctx, f.ed.ID, "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SwitchTree(ctx, tok, f.ed.ID, f.tree); err != nil {
		t.Fatal(err)
	}
	if sess, _ := s.SessionByToken(ctx, tok); !sess.Actor().CanEdit() {
		t.Fatal("an editor must be able to edit")
	}
	if _, err := s.SetMemberRole(ctx, f.owner.ID, f.tree, f.ed.ID, RoleViewer); err != nil {
		t.Fatal(err)
	}
	sess, err := s.SessionByToken(ctx, tok)
	if err != nil || sess.TreeRole != RoleViewer || sess.Actor().CanEdit() {
		t.Errorf("after demotion: %+v, %v", sess, err)
	}
}

func TestSwitchTreeAndNextSession(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	a := admin(t, s)
	bob := mustAccount(t, s, a, "bob", "").User
	t1, _ := s.CreateTree(ctx, bob.ID, "T1")
	t2, _ := s.CreateTree(ctx, bob.ID, "T2")
	tok, err := s.CreateSession(ctx, bob.ID, "x")
	if err != nil {
		t.Fatal(err)
	}
	if sess, _ := s.SessionByToken(ctx, tok); sess.TreeID != t1.ID {
		t.Fatalf("first session opens the oldest tree, got %d", sess.TreeID)
	}
	if err := s.SwitchTree(ctx, tok, bob.ID, t2.ID); err != nil {
		t.Fatal(err)
	}
	if sess, _ := s.SessionByToken(ctx, tok); sess.TreeID != t2.ID || sess.TreeRole != RoleOwner {
		t.Errorf("after switch: %+v", sess)
	}
	tok2, _ := s.CreateSession(ctx, bob.ID, "y")
	if sess, _ := s.SessionByToken(ctx, tok2); sess.TreeID != t2.ID {
		t.Errorf("new session should open the last chosen tree, got %d", sess.TreeID)
	}

	// Not a member: refused and the session stays where it is.
	admTree := mustAccount(t, s, a, "carol", "").User
	foreign, _ := s.CreateTree(ctx, admTree.ID, "Foreign")
	if err := s.SwitchTree(ctx, tok, bob.ID, foreign.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign tree: %v", err)
	}
	if sess, _ := s.SessionByToken(ctx, tok); sess.TreeID != t2.ID {
		t.Errorf("session moved to %d", sess.TreeID)
	}
	// Another user's token does not move.
	if err := s.SwitchTree(ctx, tok, admTree.ID, foreign.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign token: %v", err)
	}
	if sess, _ := s.SessionByToken(ctx, tok); sess.User.ID != bob.ID || sess.TreeID != t2.ID {
		t.Errorf("session after foreign switch: %+v", sess)
	}

	// Leaving T2: a new session opens T1, since T2 is no longer a membership.
	if _, err := s.SetMemberRole(ctx, bob.ID, t2.ID, bob.ID, RoleOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMember(ctx, bob.ID, t2.ID, MemberInput{Username: "carol", Role: RoleOwner}); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveMember(ctx, bob.ID, t2.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	tok3, _ := s.CreateSession(ctx, bob.ID, "z")
	if sess, _ := s.SessionByToken(ctx, tok3); sess.TreeID != t1.ID {
		t.Errorf("after leaving T2 a new session opens %d, want T1", sess.TreeID)
	}
}

func TestMemberChangesAreLogged(t *testing.T) {
	s := newStore(t)
	f := newFixture(t, s)
	if _, err := s.RenameTree(ctx, f.owner.ID, f.tree, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetMemberRole(ctx, f.owner.ID, f.tree, f.view.ID, RoleEditor); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveMember(ctx, f.owner.ID, f.tree, f.view.ID); err != nil {
		t.Fatal(err)
	}
	count := func(entity, action string, entityID int64) int {
		var n int
		err := s.DB.QueryRow(`
			SELECT count(*) FROM change_log WHERE tree_id = ? AND user_id = ? AND entity_type = ? AND action = ? AND entity_id = ?`,
			f.tree, f.owner.ID, entity, action, entityID).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, c := range []struct {
		entity, action string
		id             int64
		want           int
	}{
		{"tree", "create", f.tree, 1},
		{"tree", "update", f.tree, 1},
		{"member", "create", f.ed.ID, 1},
		{"member", "create", f.view.ID, 1},
		{"member", "update", f.view.ID, 1},
		{"member", "delete", f.view.ID, 1},
	} {
		if got := count(c.entity, c.action, c.id); got != c.want {
			t.Errorf("%s %s %d: %d rows, want %d", c.entity, c.action, c.id, got, c.want)
		}
	}
}
