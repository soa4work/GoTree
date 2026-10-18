package store

import (
	"database/sql"
	"errors"
	"testing"
)

const pw = "correct horse battery"

// admin returns the user created by setup.
func admin(t *testing.T, s *Store) User {
	t.Helper()
	var u User
	err := s.DB.QueryRow(`SELECT id, username, display_name, role FROM users WHERE username = 'admin'`).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func mustAccount(t *testing.T, s *Store, by User, name, role string) Account {
	t.Helper()
	a, err := s.CreateAccount(ctx, by, AccountInput{Username: name, Password: pw, Role: role})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAccountsAdminOnly(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	u := User{ID: 2, Role: "user"}
	checks := map[string]error{}
	_, checks["list"] = s.ListAccounts(ctx, u)
	_, checks["create"] = s.CreateAccount(ctx, u, AccountInput{Username: "bob", Password: pw})
	_, checks["update"] = s.UpdateAccount(ctx, u, 1, AccountUpdate{Role: "user"})
	checks["reset"] = s.ResetPassword(ctx, u, 1, pw)
	checks["delete"] = s.DeleteAccount(ctx, u, 1)
	for name, err := range checks {
		if !errors.Is(err, ErrAdminOnly) {
			t.Errorf("%s: got %v, want ErrAdminOnly", name, err)
		}
	}
}

func TestCreateAccount(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	by := admin(t, s)

	a := mustAccount(t, s, by, "bob", "")
	if a.Role != UserRoleUser || a.Trees == nil || len(a.Trees) != 0 {
		t.Errorf("new account: %+v", a)
	}
	if _, err := s.Authenticate(ctx, "bob", pw); err != nil {
		t.Errorf("the new password must work: %v", err)
	}

	_, err := s.CreateAccount(ctx, by, AccountInput{Username: "Admin", Password: pw})
	validationField(t, err, "username")
	var ve *ValidationError
	if errors.As(err, &ve) && ve.Fields["username"] != "is already taken" {
		t.Errorf("message %q", ve.Fields["username"])
	}

	_, err = s.CreateAccount(ctx, by, AccountInput{Username: "a b", Password: pw})
	validationField(t, err, "username")
	_, err = s.CreateAccount(ctx, by, AccountInput{Username: "carol", Password: "short"})
	validationField(t, err, "password")
	_, err = s.CreateAccount(ctx, by, AccountInput{Username: "carol", Password: pw, Role: "root"})
	validationField(t, err, "role")
}

func TestUpdateAccountLastAdmin(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	first := admin(t, s)
	if _, err := s.UpdateAccount(ctx, first, first.ID, AccountUpdate{Role: "user"}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting the only admin: %v", err)
	}
	second := mustAccount(t, s, first, "second", UserRoleAdmin)
	got, err := s.UpdateAccount(ctx, first, first.ID, AccountUpdate{DisplayName: " Root ", Role: "user"})
	if err != nil || got.Role != "user" || got.DisplayName != "Root" {
		t.Fatalf("demoting the first: %+v, %v", got, err)
	}
	by := User{ID: second.ID, Role: UserRoleAdmin}
	if _, err := s.UpdateAccount(ctx, by, second.ID, AccountUpdate{Role: "user"}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting the last admin: %v", err)
	}
	if _, err := s.UpdateAccount(ctx, by, 999, AccountUpdate{Role: "user"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	_, err = s.UpdateAccount(ctx, by, second.ID, AccountUpdate{Role: "root"})
	validationField(t, err, "role")
}

func TestResetPassword(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	by := admin(t, s)
	bob := mustAccount(t, s, by, "bob", "")
	tok1, err := s.CreateSession(ctx, bob.ID, "a")
	if err != nil {
		t.Fatal(err)
	}
	tok2, err := s.CreateSession(ctx, bob.ID, "b")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.ResetPassword(ctx, by, bob.ID, "a brand new password"); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{tok1, tok2} {
		if _, err := s.SessionByToken(ctx, tok); !errors.Is(err, ErrNotFound) {
			t.Errorf("session survived the reset: %v", err)
		}
	}
	if _, err := s.Authenticate(ctx, "bob", "a brand new password"); err != nil {
		t.Errorf("new password: %v", err)
	}
	if _, err := s.Authenticate(ctx, "bob", pw); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("old password: %v", err)
	}
	if err := s.ResetPassword(ctx, by, by.ID, pw); !errors.Is(err, ErrResetOwnPassword) {
		t.Errorf("own id: %v", err)
	}
	if err := s.ResetPassword(ctx, by, 999, pw); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	validationField(t, s.ResetPassword(ctx, by, bob.ID, "short"), "password")
}

func TestDeleteAccount(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	a := admin(t, s)
	if err := s.DeleteAccount(ctx, a, a.ID); !errors.Is(err, ErrDeleteSelf) {
		t.Errorf("self: %v", err)
	}
	if err := s.DeleteAccount(ctx, a, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}

	bob := mustAccount(t, s, a, "bob", "")
	tree, err := s.CreateTree(ctx, bob.ID, "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(ctx, a, bob.ID); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("last owner of a tree: %v", err)
	}
	// Bob created a person; deleting him must keep it.
	bobActor := Actor{UserID: bob.ID, TreeID: tree.ID, Role: RoleOwner}
	p := mustPerson(t, s, bobActor, "Bo", "B")
	if _, err := s.AddMember(ctx, bob.ID, tree.ID, MemberInput{Username: "admin", Role: RoleOwner}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(ctx, a, bob.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.DB.QueryRow(`SELECT count(*) FROM tree_members WHERE user_id = ?`, bob.ID).Scan(&n)
	if n != 0 {
		t.Errorf("%d memberships left", n)
	}
	var by sql.NullInt64
	if err := s.DB.QueryRow(`SELECT created_by FROM persons WHERE id = ?`, p.ID).Scan(&by); err != nil || by.Valid {
		t.Errorf("created_by = %v, %v; want NULL", by, err)
	}
}

func TestDeleteAdminRules(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	a := admin(t, s)
	b := mustAccount(t, s, a, "beta", UserRoleAdmin)
	if err := s.DeleteAccount(ctx, a, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(ctx, a, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted account: %v", err)
	}
	// A is the last admin: only a regular user could try, and is refused.
	if err := s.DeleteAccount(ctx, User{ID: 99, Role: "user"}, a.ID); !errors.Is(err, ErrAdminOnly) {
		t.Errorf("regular user: %v", err)
	}
	// Someone with a stale admin role is refused inside the transaction.
	if err := s.DeleteAccount(ctx, User{ID: 99, Role: UserRoleAdmin}, a.ID); !errors.Is(err, ErrAdminOnly) {
		t.Errorf("last admin: %v", err)
	}
}

func TestListAccounts(t *testing.T) {
	s := newStore(t)
	act := setup(t, s)
	a := admin(t, s)
	mustAccount(t, s, a, "Zed", "")
	mustAccount(t, s, a, "bob", "")
	list, err := s.ListAccounts(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].Username != "admin" || list[1].Username != "bob" || list[2].Username != "Zed" {
		t.Fatalf("order: %+v", list)
	}
	if len(list[0].Trees) != 1 || list[0].Trees[0].ID != act.TreeID || list[0].Trees[0].Role != RoleOwner {
		t.Errorf("trees of admin: %+v", list[0].Trees)
	}
	if list[0].LastSeenAt == "" {
		t.Error("the admin has a session")
	}
	if list[1].LastSeenAt != "" || list[1].Trees == nil {
		t.Errorf("fresh account: %+v", list[1])
	}
}

// TestLastAdminRace has two administrators demote each other at the same
// time; one of them must stay an administrator. The loser is refused either
// as the last administrator or as no longer an administrator.
func TestLastAdminRace(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	a := admin(t, s)
	b := mustAccount(t, s, a, "second", UserRoleAdmin).User
	errs := make(chan error, 2)
	go func() { _, err := s.UpdateAccount(ctx, a, b.ID, AccountUpdate{Role: UserRoleUser}); errs <- err }()
	go func() { _, err := s.UpdateAccount(ctx, b, a.ID, AccountUpdate{Role: UserRoleUser}); errs <- err }()
	var refused int
	for range 2 {
		if err := <-errs; errors.Is(err, ErrLastAdmin) || errors.Is(err, ErrAdminOnly) {
			refused++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM users WHERE role = 'admin'`).Scan(&n); err != nil || n != 1 || refused != 1 {
		t.Errorf("admins left: %d (%v), refused: %d; want 1 and 1", n, err, refused)
	}
}

// TestDemotedAdminLosesRightsAtOnce: a caller whose role is stale (demoted
// after their session was read) is refused inside the transaction.
func TestDemotedAdminLosesRightsAtOnce(t *testing.T) {
	s := newStore(t)
	setup(t, s)
	a := admin(t, s)
	b := mustAccount(t, s, a, "second", UserRoleAdmin).User
	if _, err := s.UpdateAccount(ctx, a, b.ID, AccountUpdate{Role: UserRoleUser}); err != nil {
		t.Fatal(err)
	}
	// b still carries Role "admin" from before the demotion.
	if _, err := s.CreateAccount(ctx, b, AccountInput{Username: "sneaky", Password: pw, Role: UserRoleAdmin}); !errors.Is(err, ErrAdminOnly) {
		t.Errorf("stale admin creates an account: %v", err)
	}
	if err := s.ResetPassword(ctx, b, a.ID, "a brand new password"); !errors.Is(err, ErrAdminOnly) {
		t.Errorf("stale admin resets a password: %v", err)
	}
}
