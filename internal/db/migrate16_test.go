package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// realisticData is a small but connected tree: two trees, people with
// several names, families with children, events with places and
// participants, sources with citations on every kind of target, media,
// heirlooms with custody, share links, research tasks and drafts. Ids are
// explicit, with gaps, as in a database that has seen deletions.
var realisticData = []string{
	`INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (1, 'owner', 'x', '', '')`,
	`INSERT INTO trees (id, name, created_at, updated_at) VALUES (1, 'Weber', '', ''), (2, 'Other', '', '')`,
	`INSERT INTO tree_members (tree_id, user_id, role) VALUES (1, 1, 'owner')`,
	`INSERT INTO places (id, tree_id, parent_id, name, created_at, updated_at) VALUES (1, 1, NULL, 'Germany', '', ''), (2, 1, 1, 'Halle', '', ''), (7, 2, NULL, 'Elsewhere', '', '')`,
	`INSERT INTO persons (id, tree_id, given_names, surname, sex, gedcom_xref, surname_key, given_key, created_at, updated_at) VALUES
		(1, 1, 'Anna', 'Weber', 'F', 'I1', x'01', x'02', '', ''),
		(2, 1, 'Karl', 'Weber', 'M', 'I2', x'01', x'03', '', ''),
		(3, 1, 'Ben', 'Weber', 'M', 'I3', x'01', x'04', '', ''),
		(5, 1, 'Eva', 'Roth', 'F', 'I5', x'05', x'06', '', ''),
		(40, 2, 'Zed', 'Other', 'M', 'I40', x'07', x'08', '', '')`,
	`INSERT INTO persons_fts (rowid, names) VALUES (1, 'Anna Weber'), (2, 'Karl Weber'), (3, 'Ben Weber'), (5, 'Eva Roth'), (40, 'Zed Other')`,
	`INSERT INTO person_names (id, person_id, type, given_names, surname) VALUES (1, 1, 'birth', 'Anna', 'Roth'), (2, 1, 'married', 'Anna', 'Weber'), (9, 3, 'aka', 'Benny', 'Weber')`,
	`INSERT INTO families (id, tree_id, partner1_id, partner2_id, union_type, created_at, updated_at) VALUES (1, 1, 2, 1, 'married', '', ''), (2, 1, 3, 5, 'partners', '', '')`,
	`INSERT INTO family_children (family_id, child_id, sort_order) VALUES (1, 3, 0)`,
	`INSERT INTO events (id, tree_id, person_id, family_id, type, date_raw, place_id, created_at, updated_at) VALUES
		(1, 1, 1, NULL, 'BIRT', '1900', 2, '', ''),
		(2, 1, 1, NULL, 'DEAT', '1980', 2, '', ''),
		(3, 1, 3, NULL, 'BIRT', '1930', NULL, '', ''),
		(4, 1, NULL, 1, 'MARR', '1925', 2, '', ''),
		(5, 1, NULL, 2, 'MARR', '1955', NULL, '', '')`,
	`INSERT INTO event_participants (event_id, person_id, role) VALUES (3, 5, 'GODP'), (1, 2, 'WITN')`,
	`INSERT INTO repositories (id, tree_id, name, created_at, updated_at) VALUES (1, 1, 'Archive', '', '')`,
	`INSERT INTO sources (id, tree_id, repository_id, title, created_at, updated_at) VALUES (1, 1, 1, 'Parish book', '', ''), (2, 1, NULL, 'Letters', '', '')`,
	`INSERT INTO citations (id, tree_id, source_id, page, created_at, updated_at) VALUES
		(1, 1, 1, '12', '', ''), (2, 1, 1, '13', '', ''), (3, 1, 2, '', '', ''), (4, 1, 2, '', '', ''), (9, 1, 1, 'orphan', '', '')`,
	`INSERT INTO heirlooms (id, tree_id, name, origin_place_id, created_at, updated_at) VALUES (1, 1, 'Ring', 2, '', '')`,
	`INSERT INTO heirloom_custody (id, heirloom_id, person_id, how) VALUES (1, 1, 2, 'inherited'), (2, 1, NULL, 'gift')`,
	// 1: person 1 and her birth; 2: only the marriage family; 3: a name;
	// 4: the heirloom; 9 has no links at all.
	`INSERT INTO citation_links (citation_id, entity_type, entity_id, field) VALUES
		(1, 'person', 1, ''), (1, 'event', 1, 'date'), (2, 'family', 1, ''),
		(3, 'name', 9, ''), (4, 'heirloom', 1, '')`,
	`INSERT INTO media (id, tree_id, sha256, mime, kind, size, created_at, updated_at) VALUES (1, 1, printf('%064d', 1), 'image/jpeg', 'image', 1, '', '')`,
	`INSERT INTO media_links (media_id, entity_type, entity_id, sort_order) VALUES (1, 'person', 1, 0), (1, 'event', 1, 1), (1, 'heirloom', 1, 2)`,
	`INSERT INTO media_regions (id, media_id, person_id, x, y, w, h) VALUES (1, 1, 1, 0.1, 0.1, 0.2, 0.2)`,
	`UPDATE persons SET portrait_media_id = 1 WHERE id = 1`,
	`INSERT INTO share_links (id, tree_id, token_hash, label, scope, root_person_id, privacy, created_at) VALUES (1, 1, x'aa', 'Anna branch', 'descendants', 1, 'deceased', '')`,
	`INSERT INTO research_tasks (id, tree_id, title, origin, origin_person_id, created_at, updated_at) VALUES (1, 1, 'Find her', 'missing:1', 2, '', '')`,
	`INSERT INTO research_log (id, tree_id, searched_on, query, result, created_at, updated_at) VALUES (1, 1, '2020-01-01', 'Weber', 'not_found', '', '')`,
	`INSERT INTO research_links (owner_type, owner_id, entity_type, entity_id) VALUES ('task', 1, 'person', 2), ('task', 1, 'place', 2)`,
	`INSERT INTO transcriptions (id, tree_id, template_key, source_id, place_id, event_id, rows_json, created_at, updated_at) VALUES
		(1, 1, 'census', 1, 2, 3, '[{"action":"person","personId":1}]', '', '')`,
	`INSERT INTO change_log (id, tree_id, entity_type, entity_id, action, at) VALUES (1, 1, 'person', 1, 'create', '')`,
}

type dbSchema struct {
	triggers, indexes, tables []string
	counts                    map[string]int
}

func snapshot(t *testing.T, conn *sql.DB) dbSchema {
	t.Helper()
	ctx := context.Background()
	s := dbSchema{counts: map[string]int{}}
	names := func(typ string) []string {
		rows, err := conn.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = ? AND name NOT LIKE 'sqlite_%' ORDER BY name`, typ)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			out = append(out, n)
		}
		return out
	}
	s.triggers = names("trigger")
	s.indexes = names("index")
	for _, n := range names("table") {
		if n != "goose_db_version" {
			s.tables = append(s.tables, n)
		}
	}
	for _, tbl := range s.tables {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM "`+tbl+`"`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		s.counts[tbl] = n
	}
	return s
}

func foreignKeyProblems(t *testing.T, conn *sql.DB) []string {
	t.Helper()
	rows, err := conn.QueryContext(context.Background(), `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var table, parent string
		var rowid, fkid sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			t.Fatal(err)
		}
		out = append(out, table+"->"+parent)
	}
	return out
}

func tableSQL(t *testing.T, conn *sql.DB, name string) string {
	t.Helper()
	var s string
	if err := conn.QueryRowContext(context.Background(), `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func scalar(t *testing.T, conn *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func has(list []string, s string) bool {
	i := sort.SearchStrings(list, s)
	return i < len(list) && list[i] == s
}

func TestMigration16KeepsRealisticData(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	p, err := newProvider(conn)
	if err != nil {
		t.Fatal(err)
	}
	// Roll back migration 16 only: the data goes into the old schema.
	if _, err := p.DownTo(ctx, 15); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range autoincrementTables {
		if strings.Contains(tableSQL(t, conn, tbl), "AUTOINCREMENT") {
			t.Fatalf("%s still has AUTOINCREMENT after Down", tbl)
		}
	}
	for _, q := range realisticData {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if bad := foreignKeyProblems(t, conn); len(bad) > 0 {
		t.Fatalf("fixture is inconsistent: %v", bad)
	}
	before := snapshot(t, conn)
	if has(before.triggers, "citation_links_drop_orphans") {
		t.Fatal("the orphan trigger exists before Up")
	}

	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, conn)

	for _, tbl := range autoincrementTables {
		if !strings.Contains(tableSQL(t, conn, tbl), "AUTOINCREMENT") {
			t.Errorf("%s has no AUTOINCREMENT after Up", tbl)
		}
	}
	// Row counts: every table is unchanged except citations, which lost
	// the one without links.
	for tbl, n := range before.counts {
		want := n
		if tbl == "citations" {
			want--
		}
		if after.counts[tbl] != want {
			t.Errorf("%s: %d rows after Up, want %d", tbl, after.counts[tbl], want)
		}
	}
	if !reflect.DeepEqual(before.tables, after.tables) {
		t.Errorf("tables changed:\n%v\n%v", before.tables, after.tables)
	}
	// Every index and trigger is back (migration 16 adds one trigger).
	if !reflect.DeepEqual(before.indexes, after.indexes) {
		t.Errorf("indexes changed:\n%v\n%v", before.indexes, after.indexes)
	}
	wantTriggers := append(append([]string{}, before.triggers...), "citation_links_drop_orphans")
	sort.Strings(wantTriggers)
	if !reflect.DeepEqual(wantTriggers, after.triggers) {
		t.Errorf("triggers:\n got %v\nwant %v", after.triggers, wantTriggers)
	}
	for _, idx := range []string{"persons_tree_sort", "persons_tree_name", "events_person", "citations_source", "heirloom_custody_person", "research_tasks_origin"} {
		if !has(after.indexes, idx) {
			t.Errorf("index %s missing", idx)
		}
	}
	if bad := foreignKeyProblems(t, conn); len(bad) > 0 {
		t.Errorf("foreign key violations after Up: %v", bad)
	}
	if n := scalar(t, conn, `PRAGMA foreign_keys`); n != 1 {
		t.Errorf("foreign_keys = %d after Up", n)
	}
	// Even a connection that was in the pool during the migration enforces them.
	if _, err := conn.ExecContext(ctx, `INSERT INTO family_children (family_id, child_id) VALUES (99, 99)`); err == nil {
		t.Error("foreign keys are not enforced after Up")
	}

	// Values and ids survived.
	if n := scalar(t, conn, `SELECT count(*) FROM persons WHERE id IN (1,2,3,5,40) AND given_names <> ''`); n != 5 {
		t.Errorf("persons: %d", n)
	}
	if n := scalar(t, conn, `SELECT portrait_media_id FROM persons WHERE id = 1`); n != 1 {
		t.Errorf("portrait: %d", n)
	}
	if n := scalar(t, conn, `SELECT person_id FROM events WHERE id = 3`); n != 3 {
		t.Errorf("event 3 person: %d", n)
	}
	if n := scalar(t, conn, `SELECT place_id FROM events WHERE id = 1`); n != 2 {
		t.Errorf("event place: %d", n)
	}
	if n := scalar(t, conn, `SELECT parent_id FROM places WHERE id = 2`); n != 1 {
		t.Errorf("place parent: %d", n)
	}
	if n := scalar(t, conn, `SELECT child_id FROM family_children WHERE family_id = 1`); n != 3 {
		t.Errorf("family child: %d", n)
	}
	if n := scalar(t, conn, `SELECT person_id FROM heirloom_custody WHERE id = 1`); n != 2 {
		t.Errorf("custody person: %d", n)
	}
	if n := scalar(t, conn, `SELECT root_person_id FROM share_links WHERE id = 1`); n != 1 {
		t.Errorf("share root: %d", n)
	}
	if n := scalar(t, conn, `SELECT origin_person_id FROM research_tasks WHERE id = 1`); n != 2 {
		t.Errorf("task origin: %d", n)
	}
	if n := scalar(t, conn, `SELECT count(*) FROM persons_fts WHERE persons_fts MATCH 'Weber'`); n != 3 {
		t.Errorf("search index: %d matches for Weber", n)
	}
	if n := scalar(t, conn, `SELECT count(*) FROM citations WHERE id = 9`); n != 0 {
		t.Error("the citation without links survived")
	}
	if n := scalar(t, conn, `SELECT count(*) FROM citations WHERE id IN (1,2,3,4)`); n != 4 {
		t.Errorf("linked citations: %d", n)
	}

	// The old triggers work: deleting person 1 removes what hangs on her,
	// and the citation that supported only her and her birth.
	if _, err := conn.ExecContext(ctx, `DELETE FROM persons WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, conn, `SELECT count(*) FROM events WHERE person_id = 1`); n != 0 {
		t.Errorf("%d events of the deleted person", n)
	}
	if n := scalar(t, conn, `SELECT count(*) FROM citation_links WHERE (entity_type = 'person' AND entity_id = 1) OR (entity_type = 'event' AND entity_id IN (1, 2))`); n != 0 {
		t.Errorf("%d citation links of the deleted person", n)
	}
	if n := scalar(t, conn, `SELECT count(*) FROM media_links WHERE entity_type IN ('person', 'event') AND entity_id = 1`); n != 0 {
		t.Errorf("%d media links of the deleted person", n)
	}
	if n := scalar(t, conn, `SELECT count(*) FROM citations WHERE id = 1`); n != 0 {
		t.Error("the citation of the deleted person stays")
	}
	if n := scalar(t, conn, `SELECT count(*) FROM citations WHERE id IN (2,3,4)`); n != 3 {
		t.Error("a citation with other links was deleted")
	}
	// A citation goes with its last link, whichever way that goes.
	if _, err := conn.ExecContext(ctx, `DELETE FROM families WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, conn, `SELECT count(*) FROM citations WHERE id = 2`); n != 0 {
		t.Error("the family citation stays after the family is gone")
	}
	if bad := foreignKeyProblems(t, conn); len(bad) > 0 {
		t.Errorf("foreign key violations after deletions: %v", bad)
	}

	// No id reuse.
	res, err := conn.ExecContext(ctx, `INSERT INTO persons (tree_id, created_at, updated_at) VALUES (1, '', '')`)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := res.LastInsertId()
	if _, err := conn.ExecContext(ctx, `DELETE FROM persons WHERE id = ?`, n); err != nil {
		t.Fatal(err)
	}
	res, err = conn.ExecContext(ctx, `INSERT INTO persons (tree_id, created_at, updated_at) VALUES (1, '', '')`)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := res.LastInsertId(); m <= n {
		t.Errorf("id %d reused after deleting %d", m, n)
	}
}

// Down must give the old schema back with all data, and Up must work again.
func TestMigration16DownAndUpAgain(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	p, err := newProvider(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 15); err != nil {
		t.Fatal(err)
	}
	for _, q := range realisticData {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	up := snapshot(t, conn)

	if _, err := p.DownTo(ctx, 15); err != nil {
		t.Fatal(err)
	}
	down := snapshot(t, conn)
	for _, tbl := range autoincrementTables {
		if strings.Contains(tableSQL(t, conn, tbl), "AUTOINCREMENT") {
			t.Errorf("%s has AUTOINCREMENT after Down", tbl)
		}
	}
	if has(down.triggers, "citation_links_drop_orphans") {
		t.Error("the orphan trigger survives Down")
	}
	wantTriggers := []string{}
	for _, tr := range up.triggers {
		if tr != "citation_links_drop_orphans" {
			wantTriggers = append(wantTriggers, tr)
		}
	}
	if !reflect.DeepEqual(wantTriggers, down.triggers) {
		t.Errorf("triggers after Down:\n got %v\nwant %v", down.triggers, wantTriggers)
	}
	if !reflect.DeepEqual(up.indexes, down.indexes) {
		t.Errorf("indexes after Down:\n got %v\nwant %v", down.indexes, up.indexes)
	}
	if !reflect.DeepEqual(up.counts, down.counts) {
		t.Errorf("row counts after Down: %v, want %v", down.counts, up.counts)
	}
	if bad := foreignKeyProblems(t, conn); len(bad) > 0 {
		t.Errorf("foreign key violations after Down: %v", bad)
	}
	if n := scalar(t, conn, `PRAGMA foreign_keys`); n != 1 {
		t.Errorf("foreign_keys = %d after Down", n)
	}
	// The old triggers still work after Down.
	if _, err := conn.ExecContext(ctx, `DELETE FROM persons WHERE id = 3`); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, conn, `SELECT count(*) FROM events WHERE person_id = 3`); n != 0 {
		t.Error("events of the deleted person remain after Down")
	}

	// Up again on the rolled-back database, then once more (a no-op).
	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if bad := foreignKeyProblems(t, conn); len(bad) > 0 {
		t.Errorf("foreign key violations after the second Up: %v", bad)
	}
	again := snapshot(t, conn)
	if !has(again.triggers, "citation_links_drop_orphans") {
		t.Error("orphan trigger missing after the second Up")
	}
	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	// Idempotent rebuild: a second pass changes nothing.
	if err := withoutForeignKeys(ctx, conn, func(tx *sql.Tx) error {
		for _, tbl := range autoincrementTables {
			if err := rebuild(ctx, tx, tbl, true); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	provider, err := newProvider(conn)
	if err != nil {
		t.Fatal(err)
	}
	sources := provider.ListSources()
	newest := sources[len(sources)-1].Version
	if v, err := SchemaVersion(ctx, conn); err != nil || v != newest {
		t.Errorf("schema version %d, %v; want %d", v, err, newest)
	}
}

// A fresh database passes through migration 16 with no rows at all.
func TestMigration16OnEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if bad := foreignKeyProblems(t, conn); len(bad) > 0 {
		t.Errorf("foreign key violations: %v", bad)
	}
	if !has(snapshot(t, conn).triggers, "citation_links_drop_orphans") {
		t.Error("orphan trigger missing")
	}
}
