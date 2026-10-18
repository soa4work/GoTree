package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// storeMessages returns every validation message written in internal/store:
// the message argument of validator.add/check calls and the values of
// ValidationError{Fields: ...} literals. A message built at run time keeps
// its literal parts and shows each dynamic part as "{}" (Sprintf verbs too).
func storeMessages(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("../store/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no store files: %v", err)
	}
	out := map[string]bool{}
	var text func(e ast.Expr) (string, bool)
	text = func(e ast.Expr) (string, bool) {
		switch e := e.(type) {
		case *ast.BasicLit:
			if e.Kind == token.STRING {
				s, _ := strconv.Unquote(e.Value)
				return s, true
			}
		case *ast.BinaryExpr:
			l, lok := text(e.X)
			r, rok := text(e.Y)
			if !lok {
				l = "{}"
			}
			if !rok {
				r = "{}"
			}
			return l + r, lok || rok
		case *ast.CallExpr:
			if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Sprintf" && len(e.Args) > 0 {
				if f, ok := text(e.Args[0]); ok {
					return strings.NewReplacer("%d", "{}", "%s", "{}", "%v", "{}").Replace(f), true
				}
			}
		}
		return "", false
	}
	add := func(e ast.Expr) {
		if s, ok := text(e); ok {
			out[s] = true
		}
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch {
				case sel.Sel.Name == "add" && len(n.Args) == 2:
					add(n.Args[1])
				case sel.Sel.Name == "check" && len(n.Args) == 3:
					add(n.Args[2])
				}
			case *ast.CompositeLit:
				id, ok := n.Type.(*ast.Ident)
				if !ok || id.Name != "ValidationError" {
					return true
				}
				ast.Inspect(n, func(m ast.Node) bool {
					if kv, ok := m.(*ast.KeyValueExpr); ok {
						if _, isLit := kv.Key.(*ast.BasicLit); isLit {
							add(kv.Value)
						}
					}
					return true
				})
			}
			return true
		})
	}
	return out
}

// pinned maps every message the store can produce to the code the client gets.
// A reworded or new store message fails TestStoreMessagesAreClassified until it
// is listed here, so it cannot silently degrade to FieldInvalid.
// List a message as FieldInvalid only on purpose: the client then shows the
// English text.
var pinned = map[string]string{
	"a citation is not attached here":                                      "citation_not_attached",
	"an account with this username does not exist":                         "not_found",
	"a cited {} does not exist":                                            "not_found",
	"a custom role needs a name":                                           "name_required",
	"a date is too long":                                                   "too_long",
	"a family event needs two partner roles and no principal":              "roles_invalid",
	"a holder does not exist":                                              "not_found",
	"a linked {} does not exist":                                           "not_found",
	"a living person has no death":                                         "living_no_death",
	"a name needs at least a given name, surname or nickname":              "name_part_required",
	"a new person needs a name":                                            "name_required",
	"a participant does not exist":                                         "not_found",
	"a person cannot be a child of themselves or of their own descendant":  "cycle",
	"a person cannot be a parent of their own ancestor":                    "cycle",
	"a person cannot be their own partner":                                 "self_reference",
	"a person cannot be their own relative":                                "self_reference",
	"a place cannot be inside itself":                                      "self_reference",
	"a place is empty":                                                     "place_empty",
	"a portrait must be a photo":                                           "photo_required",
	"a record can have at most {} rows":                                    "too_many_rows",
	"a template needs a name column":                                       "name_required",
	"a value is too long":                                                  "too_long",
	"an event belongs to exactly one person or one family":                 "event_owner",
	"both parents in this family are already known":                        "parents_known",
	"does not exist":                                                       "not_found",
	"every role needs a name of at most 60 characters":                     "name_length",
	"every role needs its own key":                                         "roles_invalid",
	"exactly one row needs the main role":                                  "rows_invalid",
	"field name is too long":                                               "too_long",
	"give either an existing person or a new one":                          "person_or_new",
	"give the feed a name of at most 120 characters":                       "name_length",
	"give the link a name of at most 120 characters":                       "name_length",
	"give the object a name of at most 200 characters":                     "name_length",
	"give the template a name of at most 120 characters":                   "name_length",
	"is already a member of this tree":                                     "already_member",
	"is already taken":                                                     "taken",
	"is not a face tag on this photo":                                      "not_face_tag",
	"is not a suggestion":                                                  "not_suggestion",
	"is not one of this person's families":                                 "family_choice",
	"is not your current password":                                         "wrong_password",
	"is required":                                                          "required",
	"is required for custom events":                                        "custom_label_required",
	"is too long":                                                          "too_long",
	"item {} is not a canonical GEDCOM date":                               "not_canonical_date",
	"latitude and longitude must be given together":                        "coords_together",
	"month and day are out of range":                                       "date_out_of_range",
	"must be M, F, U or X":                                                 "invalid_choice",
	"must be a GEDCOM tag such as BIRT, or a custom tag starting with _":   "invalid_tag",
	"must be a GEDCOM tag such as CENS, or a custom tag starting with _":   "invalid_tag",
	"must be a date":                                                       "invalid_date",
	"must be accepted, disputed or disproven":                              "invalid_choice",
	"must be all, exclude-living or name-only":                             "invalid_choice",
	"must be admin or user":                                                "invalid_choice",
	"must be at least 10 characters":                                       "password_too_short",
	"must be at most 72 bytes":                                             "password_too_long",
	"must be between -180 and 180":                                         "out_of_range",
	"must be between -90 and 90":                                           "out_of_range",
	"must be between 0 and 3":                                              "out_of_range",
	"must be deceased or living_names":                                     "invalid_choice",
	"must be empty or replace":                                             "invalid_choice",
	"must be found, not_found or partial":                                  "invalid_choice",
	"must be low, normal or high":                                          "invalid_choice",
	"must be married, partners or unknown":                                 "invalid_choice",
	"must be open, in_progress or done":                                    "invalid_choice",
	"must be owner, editor or viewer":                                      "invalid_choice",
	"must be parent, partner, child or sibling":                            "invalid_choice",
	"must be person, event, family or source":                              "invalid_choice",
	"must be tree or descendants":                                          "invalid_choice",
	"must not be in the past":                                              "date_in_past",
	"must not contain a comma; use the parent place for the larger region": "place_comma",
	"must start with http:// or https://":                                  "invalid_url",
	"name the holder: pick a person or say who in the notes":               "holder_required",
	"name the kind of record, e.g. “School register”":                      "record_title",
	"names a person that does not exist":                                   "not_found",
	"only photos can have face tags":                                       "photo_required",
	"pick the person":                                                      "pick_person",
	"pick the person whose descendants to share":                           "pick_person",
	"pick the source the record comes from":                                "pick_source",
	"say what to find out, in at most 300 characters":                      "text_length",
	"say what you searched for, in at most 500 characters":                 "text_length",
	"say why the fact is disputed or disproven":                            "reason_required",
	"say why the name is disputed or disproven":                            "reason_required",
	"the box must lie within the image":                                    "outside_image",
	"the event needs exactly one principal role":                           "roles_invalid",
	"the line is too long":                                                 "too_long",
	"the partners must be two different people":                            "self_reference",
	"the person does not exist":                                            "not_found",
	"the principal cannot also be a participant":                           "self_reference",
	"the record needs both partners":                                       "rows_invalid",
	"the same fact is listed twice":                                        "duplicate",
	"the same person is listed twice with the same role":                   "duplicate",
	"this event belongs to a family":                                       "family_event",
	"this person has several families; choose one":                         "family_choice",
	"too many holders":                                                     "too_many",
	"too many links":                                                       "too_many",
	"too many names":                                                       "too_many",
	"unknown action {}":                                                    "unknown_value",
	"unknown column {}":                                                    "unknown_value",
	"unknown kind":                                                         "unknown",
	"unknown kind {}":                                                      "unknown_value",
	"unknown name type {}":                                                 "unknown_value",
	"unknown or repeated column {}":                                        "unknown_value",
	"unknown participant role {}":                                          "unknown_value",
	"unknown relation":                                                     "unknown",
	"unknown role {}":                                                      "unknown_value",
	"unknown status {}":                                                    "unknown_value",
	"unknown template":                                                     "unknown",
	"unknown way of getting it: {}":                                        "unknown_value",
	"use 2–64 letters, digits, dots, dashes or underscores":                "username_format",
}

func TestStoreMessagesAreClassified(t *testing.T) {
	found := storeMessages(t)
	var msgs []string
	for m := range found {
		msgs = append(msgs, m)
	}
	sort.Strings(msgs)
	for _, m := range msgs {
		want, ok := pinned[m]
		if !ok {
			t.Errorf("unpinned store message %q (classifies as %q)", m, classifyField(strings.ReplaceAll(m, "{}", "7")).Code)
			continue
		}
		if got := classifyField(strings.ReplaceAll(m, "{}", "7")).Code; got != want {
			t.Errorf("%q: code %q, want %q", m, got, want)
		}
	}
	for m := range pinned {
		if !found[m] {
			t.Errorf("pinned message %q no longer exists in the store", m)
		}
	}
}
