package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/praetorianer777/gotree/internal/gedcom"
	"github.com/praetorianer777/gotree/internal/gendate"
)

// Privacy modes for exports.
const (
	PrivacyAll            = "all"
	PrivacyExcludeLiving  = "exclude-living"
	PrivacyLivingNameOnly = "name-only"
)

// ExportOptions selects what an export contains.
type ExportOptions struct {
	Version7 bool
	Privacy  string
	// WithMedia adds OBJE records with FILE paths "media/<sha256><ext>",
	// for a GEDZIP archive that contains those files.
	WithMedia bool
	// AppVersion is written to HEAD.SOUR.VERS.
	AppVersion string
}

// MediaFile is a file an export refers to, for packing into an archive.
type MediaFile struct {
	SHA256 string
	Path   string
}

// Export is a GEDCOM file as records, ready for gedcom.Write.
type Export struct {
	Records []*gedcom.Node
	Media   []MediaFile
	// Counts holds the exported records by kind.
	Counts map[string]int
}

// extensionTags are GoTree's own tags, declared in a GEDCOM 7 header.
var extensionTags = []string{"_CUST", "_DESC", "_FREL", "_FROM", "_HEIRLOOM", "_LOC", "_MILT", "_MREL", "_STAT", "_TO"}

const extensionDocs = "https://github.com/praetorianer777/GoTree/blob/main/docs/gedcom-extensions.md#"

type exportPerson struct {
	id                                       int64
	given, surname, prefix, suffix, nickname string
	sex, notes, xref, extra                  string
	living                                   bool
}

type exportName struct {
	personID                                      int64
	id                                            int64
	typ, given, surname, prefix, suffix, nickname string
	status, statusReason                          string
}

type exportEvent struct {
	id                                 int64
	personID, familyID, placeID        sql.NullInt64
	typ, label, raw, date, desc, notes string
	status, statusReason, extra        string
}

type exportFamily struct {
	id       int64
	p1, p2   sql.NullInt64
	union    string
	notes    string
	xref     string
	extra    string
	children []exportChild
}

type exportChild struct {
	personID   int64
	rel1, rel2 string
}

type exportCitation struct {
	id                int64
	sourceID          int64
	page, text, notes string
	quality           sql.NullInt64
	extra             string
}

type exporter struct {
	s   *Store
	a   Actor
	opt ExportOptions
	out Export
	now time.Time

	persons  []exportPerson
	included map[int64]bool // persons in the export
	nameOnly map[int64]bool // living persons reduced to their name
	names    map[int64][]exportName
	events   []exportEvent
	families []exportFamily
	places   map[int64]placeInfo
	cites    map[string][]exportCitation // "entity:id" → citations
	parts    map[int64][]participantRow  // event id → participants
	media    map[string][]int64          // "entity:id" → media ids
	xrefMap  map[string]string           // original GEDCOM xref → exported xref
	dangling int
}

type placeInfo struct {
	fullName string
	lat, lng sql.NullFloat64
}

type participantRow struct {
	personID int64
	role     string
}

// ExportGEDCOM builds the records of a GEDCOM file for the actor's tree.
func (s *Store) ExportGEDCOM(ctx context.Context, a Actor, opt ExportOptions) (Export, error) {
	if opt.Privacy == "" {
		opt.Privacy = PrivacyAll
	}
	if !oneOf(opt.Privacy, PrivacyAll, PrivacyExcludeLiving, PrivacyLivingNameOnly) {
		return Export{}, &ValidationError{Fields: map[string]string{"privacy": "must be all, exclude-living or name-only"}}
	}
	ex := &exporter{
		s: s, a: a, opt: opt, now: s.Now(), out: Export{Counts: map[string]int{}},
		included: map[int64]bool{}, nameOnly: map[int64]bool{}, names: map[int64][]exportName{},
		places: map[int64]placeInfo{}, cites: map[string][]exportCitation{}, parts: map[int64][]participantRow{},
		media: map[string][]int64{}, xrefMap: map[string]string{},
	}
	if err := ex.load(ctx); err != nil {
		return Export{}, err
	}
	if err := ex.build(ctx); err != nil {
		return Export{}, err
	}
	return ex.out, nil
}

func (ex *exporter) load(ctx context.Context) error {
	db, a := ex.s.DB, ex.a
	living, err := ex.s.livingAll(ctx, a)
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, given_names, surname, name_prefix, name_suffix, nickname, sex, notes, gedcom_xref, extra_json
		FROM persons WHERE tree_id = ? ORDER BY id`, a.TreeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p exportPerson
		if err := rows.Scan(&p.id, &p.given, &p.surname, &p.prefix, &p.suffix, &p.nickname, &p.sex, &p.notes, &p.xref, &p.extra); err != nil {
			rows.Close()
			return err
		}
		p.living = living[p.id]
		switch {
		case p.living && ex.opt.Privacy == PrivacyExcludeLiving:
			continue
		case p.living && ex.opt.Privacy == PrivacyLivingNameOnly:
			ex.nameOnly[p.id] = true
		}
		ex.included[p.id] = true
		ex.persons = append(ex.persons, p)
		if p.xref != "" {
			ex.xrefMap[p.xref] = personXref(p.id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = db.QueryContext(ctx, `
		SELECT n.person_id, n.id, n.type, n.given_names, n.surname, n.name_prefix, n.name_suffix, n.nickname, n.status, n.status_reason
		FROM person_names n JOIN persons p ON p.id = n.person_id WHERE p.tree_id = ? ORDER BY n.person_id, n.sort_order, n.id`, a.TreeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var n exportName
		if err := rows.Scan(&n.personID, &n.id, &n.typ, &n.given, &n.surname, &n.prefix, &n.suffix, &n.nickname, &n.status, &n.statusReason); err != nil {
			rows.Close()
			return err
		}
		ex.names[n.personID] = append(ex.names[n.personID], n)
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `
		SELECT id, person_id, family_id, place_id, type, custom_label, date_raw, date, description, notes, status, status_reason, extra_json
		FROM events WHERE tree_id = ? ORDER BY sort_order, date_sort IS NULL, date_sort, id`, a.TreeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var e exportEvent
		if err := rows.Scan(&e.id, &e.personID, &e.familyID, &e.placeID, &e.typ, &e.label, &e.raw, &e.date, &e.desc, &e.notes,
			&e.status, &e.statusReason, &e.extra); err != nil {
			rows.Close()
			return err
		}
		ex.events = append(ex.events, e)
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `
		SELECT f.id, f.partner1_id, f.partner2_id, f.union_type, f.notes, f.gedcom_xref, f.extra_json
		FROM families f WHERE f.tree_id = ? ORDER BY f.id`, a.TreeID)
	if err != nil {
		return err
	}
	famIndex := map[int64]int{}
	for rows.Next() {
		var f exportFamily
		if err := rows.Scan(&f.id, &f.p1, &f.p2, &f.union, &f.notes, &f.xref, &f.extra); err != nil {
			rows.Close()
			return err
		}
		famIndex[f.id] = len(ex.families)
		ex.families = append(ex.families, f)
		if f.xref != "" {
			ex.xrefMap[f.xref] = familyXref(f.id)
		}
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `
		SELECT c.family_id, c.child_id, c.relation_partner1, c.relation_partner2
		FROM family_children c JOIN families f ON f.id = c.family_id WHERE f.tree_id = ? ORDER BY c.family_id, c.sort_order, c.child_id`, a.TreeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var fid int64
		var c exportChild
		if err := rows.Scan(&fid, &c.personID, &c.rel1, &c.rel2); err != nil {
			rows.Close()
			return err
		}
		f := &ex.families[famIndex[fid]]
		f.children = append(f.children, c)
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, placeFullNameCTE+`
		SELECT p.id, f.full_name, p.lat, p.lng FROM places p JOIN full_names f ON f.id = p.id WHERE p.tree_id = ?`, a.TreeID, a.TreeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var pi placeInfo
		if err := rows.Scan(&id, &pi.fullName, &pi.lat, &pi.lng); err != nil {
			rows.Close()
			return err
		}
		ex.places[id] = pi
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `
		-- GEDCOM cites a fact as a whole; a citation of its date and of its
		-- place is one SOUR.
		SELECT DISTINCT l.entity_type, l.entity_id, c.id, c.source_id, c.page, c.text, c.notes, c.quality, c.extra_json
		FROM citation_links l JOIN citations c ON c.id = l.citation_id
		WHERE c.tree_id = ? ORDER BY c.id`, a.TreeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var typ string
		var eid int64
		var c exportCitation
		if err := rows.Scan(&typ, &eid, &c.id, &c.sourceID, &c.page, &c.text, &c.notes, &c.quality, &c.extra); err != nil {
			rows.Close()
			return err
		}
		key := fmt.Sprintf("%s:%d", typ, eid)
		ex.cites[key] = append(ex.cites[key], c)
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `
		SELECT p.event_id, p.person_id, p.role FROM event_participants p JOIN events e ON e.id = p.event_id WHERE e.tree_id = ?`, a.TreeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var eid int64
		var pr participantRow
		if err := rows.Scan(&eid, &pr.personID, &pr.role); err != nil {
			rows.Close()
			return err
		}
		ex.parts[eid] = append(ex.parts[eid], pr)
	}
	rows.Close()

	if ex.opt.WithMedia {
		rows, err = db.QueryContext(ctx, `
			SELECT l.entity_type, l.entity_id, l.media_id FROM media_links l JOIN media m ON m.id = l.media_id
			WHERE m.tree_id = ? ORDER BY l.sort_order, l.media_id`, a.TreeID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var typ string
			var eid, mid int64
			if err := rows.Scan(&typ, &eid, &mid); err != nil {
				rows.Close()
				return err
			}
			key := fmt.Sprintf("%s:%d", typ, eid)
			ex.media[key] = append(ex.media[key], mid)
		}
		rows.Close()
	}

	for _, q := range []struct{ query, prefix string }{
		{`SELECT id, gedcom_xref FROM sources WHERE tree_id = ? AND gedcom_xref <> ''`, "S"},
		{`SELECT id, gedcom_xref FROM repositories WHERE tree_id = ? AND gedcom_xref <> ''`, "R"},
	} {
		rows, err := db.QueryContext(ctx, q.query, a.TreeID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var xref string
			if err := rows.Scan(&id, &xref); err != nil {
				rows.Close()
				return err
			}
			ex.xrefMap[xref] = fmt.Sprintf("%s%d", q.prefix, id)
		}
		rows.Close()
	}
	return nil
}

func personXref(id int64) string { return fmt.Sprintf("I%d", id) }
func familyXref(id int64) string { return fmt.Sprintf("F%d", id) }

func node(tag, value string, children ...*gedcom.Node) *gedcom.Node {
	return &gedcom.Node{Tag: tag, Value: value, Children: children}
}

func pointer(tag, xref string) *gedcom.Node {
	return &gedcom.Node{Tag: tag, Pointer: xref}
}

// add appends a child when it has a value or children.
func add(n *gedcom.Node, tag, value string) {
	if strings.TrimSpace(value) != "" {
		n.Children = append(n.Children, node(tag, value))
	}
}

func (ex *exporter) build(ctx context.Context) error {
	v := "5.5.1"
	if ex.opt.Version7 {
		v = "7.0"
	}
	head := node("HEAD", "")
	gedc := node("GEDC", "", node("VERS", v))
	if !ex.opt.Version7 {
		gedc.Children = append(gedc.Children, node("FORM", "LINEAGE-LINKED"))
	}
	head.Children = append(head.Children, gedc)
	if ex.opt.Version7 {
		schma := node("SCHMA", "")
		for _, t := range extensionTags {
			schma.Children = append(schma.Children, node("TAG", t+" "+extensionDocs+strings.ToLower(t)))
		}
		head.Children = append(head.Children, schma)
	} else {
		head.Children = append(head.Children, node("CHAR", "UTF-8"))
	}
	sour := node("SOUR", "GOTREE", node("NAME", "GoTree"))
	add(sour, "VERS", ex.opt.AppVersion)
	head.Children = append(head.Children, sour, node("DATE", strings.ToUpper(ex.now.Format("2 Jan 2006"))), pointer("SUBM", "U1"))

	var submitterName string
	_ = ex.s.DB.QueryRowContext(ctx, `SELECT coalesce(nullif(display_name, ''), username) FROM users WHERE id = ?`, ex.a.UserID).Scan(&submitterName)
	if submitterName == "" {
		submitterName = "GoTree"
	}
	subm := &gedcom.Node{Xref: "U1", Tag: "SUBM", Children: []*gedcom.Node{node("NAME", submitterName)}}

	records := []*gedcom.Node{head, subm}
	partnerOf := map[int64][]int64{}
	childOf := map[int64][]exportChildRef{}
	for _, f := range ex.families {
		for _, p := range []sql.NullInt64{f.p1, f.p2} {
			if p.Valid && ex.included[p.Int64] {
				partnerOf[p.Int64] = append(partnerOf[p.Int64], f.id)
			}
		}
		for _, c := range f.children {
			if ex.included[c.personID] {
				childOf[c.personID] = append(childOf[c.personID], exportChildRef{f.id, c.rel1, c.rel2})
			}
		}
	}
	eventsOf := map[string][]exportEvent{}
	for _, e := range ex.events {
		if e.personID.Valid {
			eventsOf[fmt.Sprintf("person:%d", e.personID.Int64)] = append(eventsOf[fmt.Sprintf("person:%d", e.personID.Int64)], e)
		} else {
			eventsOf[fmt.Sprintf("family:%d", e.familyID.Int64)] = append(eventsOf[fmt.Sprintf("family:%d", e.familyID.Int64)], e)
		}
	}

	for _, p := range ex.persons {
		records = append(records, ex.personRecord(p, eventsOf[fmt.Sprintf("person:%d", p.id)], partnerOf[p.id], childOf[p.id]))
		ex.out.Counts["persons"]++
	}
	for _, f := range ex.families {
		if rec := ex.familyRecord(f, eventsOf[fmt.Sprintf("family:%d", f.id)]); rec != nil {
			records = append(records, rec)
			ex.out.Counts["families"]++
		}
	}
	srcs, err := ex.sourceRecords(ctx)
	if err != nil {
		return err
	}
	records = append(records, srcs...)
	repos, err := ex.repositoryRecords(ctx)
	if err != nil {
		return err
	}
	records = append(records, repos...)
	heirlooms, err := ex.heirloomRecords(ctx)
	if err != nil {
		return err
	}
	records = append(records, heirlooms...)
	if ex.opt.WithMedia {
		objs, err := ex.mediaRecords(ctx)
		if err != nil {
			return err
		}
		records = append(records, objs...)
	}
	ex.out.Records = records
	return nil
}

type exportChildRef struct {
	familyID   int64
	rel1, rel2 string
}

func nameValue(given, surname, suffix string) string {
	v := strings.TrimSpace(given)
	v = strings.TrimSpace(v + " /" + surname + "/")
	if suffix != "" {
		v += " " + suffix
	}
	return v
}

var nameTypes551 = map[string]string{"birth": "birth", "married": "married", "aka": "aka", "immigrant": "immigrant", "religious": "religious", "other": "other"}

func (ex *exporter) nameNode(typ, given, surname, prefix, suffix, nickname string) *gedcom.Node {
	n := node("NAME", nameValue(given, surname, suffix))
	if typ != "" {
		if ex.opt.Version7 {
			switch typ {
			case "birth", "married", "aka", "immigrant":
				n.Children = append(n.Children, node("TYPE", strings.ToUpper(typ)))
			default:
				n.Children = append(n.Children, node("TYPE", "OTHER", node("PHRASE", typ)))
			}
		} else {
			n.Children = append(n.Children, node("TYPE", nameTypes551[typ]))
		}
	}
	add(n, "NPFX", prefix)
	add(n, "GIVN", given)
	add(n, "NICK", nickname)
	add(n, "SURN", surname)
	add(n, "NSFX", suffix)
	return n
}

func (ex *exporter) personRecord(p exportPerson, events []exportEvent, partnerFams []int64, childFams []exportChildRef) *gedcom.Node {
	r := &gedcom.Node{Xref: personXref(p.id), Tag: "INDI"}
	r.Children = append(r.Children, ex.nameNode("", p.given, p.surname, p.prefix, p.suffix, p.nickname))
	short := ex.nameOnly[p.id]
	if !short {
		for _, n := range ex.names[p.id] {
			nn := ex.nameNode(n.typ, n.given, n.surname, n.prefix, n.suffix, n.nickname)
			ex.status(nn, n.status, n.statusReason)
			ex.citations(nn, "name", n.id)
			r.Children = append(r.Children, nn)
		}
	}
	if p.sex != "U" || !ex.opt.Version7 {
		sex := p.sex
		if sex == "X" && !ex.opt.Version7 {
			sex = "U" // 5.5.1 knows only M, F and U
		}
		r.Children = append(r.Children, node("SEX", sex))
	}
	if !short {
		for _, e := range events {
			r.Children = append(r.Children, ex.eventNode(e))
		}
		ex.participations(r, p.id)
		add(r, "NOTE", p.notes)
		ex.citations(r, "person", p.id)
		ex.mediaRefs(r, "person", p.id)
	}
	for _, c := range childFams {
		famc := pointer("FAMC", familyXref(c.familyID))
		if pedi := pedigree(c.rel1, c.rel2, ex.opt.Version7); pedi != "" {
			famc.Children = append(famc.Children, node("PEDI", pedi))
		}
		r.Children = append(r.Children, famc)
	}
	for _, f := range partnerFams {
		r.Children = append(r.Children, pointer("FAMS", familyXref(f)))
	}
	if !short {
		r.Children = append(r.Children, ex.kept(p.extra)...)
	}
	return r
}

// participations writes the shared events a person takes part in as 5.5.1
// ASSO lines on the person; GEDCOM 7 has them on the event itself.
func (ex *exporter) participations(r *gedcom.Node, personID int64) {
	if ex.opt.Version7 {
		return
	}
	for _, e := range ex.events {
		if !e.personID.Valid || e.personID.Int64 != personID {
			continue
		}
		for _, p := range ex.parts[e.id] {
			if !ex.included[p.personID] {
				continue
			}
			r.Children = append(r.Children, pointer("ASSO", personXref(p.personID)))
			asso := r.Children[len(r.Children)-1]
			asso.Children = append(asso.Children, node("RELA", fmt.Sprintf("%s (%s)", p.role, e.typ)))
		}
	}
}

// pedigree is the FAMC.PEDI value when both parents relate the same way.
func pedigree(rel1, rel2 string, v7 bool) string {
	if rel1 != rel2 || rel1 == "birth" {
		return ""
	}
	switch rel1 {
	case "adopted", "foster", "sealing":
		if v7 {
			return strings.ToUpper(rel1)
		}
		return rel1
	}
	return ""
}

var attributeTypes = attributeTags

func (ex *exporter) eventNode(e exportEvent) *gedcom.Node {
	n := node(e.typ, "")
	if attributeTypes[e.typ] || e.typ == "EVEN" {
		n.Value = e.desc
	}
	if e.typ == "EVEN" || e.typ == "FACT" {
		add(n, "TYPE", e.label)
	} else if !attributeTypes[e.typ] && e.desc != "" {
		add(n, "TYPE", e.desc)
	}
	ex.date(n, e.raw)
	if e.placeID.Valid {
		if pi, ok := ex.places[e.placeID.Int64]; ok {
			plac := node("PLAC", pi.fullName)
			if pi.lat.Valid && pi.lng.Valid {
				plac.Children = append(plac.Children, node("MAP", "",
					node("LATI", coord(pi.lat.Float64, "N", "S")), node("LONG", coord(pi.lng.Float64, "E", "W"))))
			}
			n.Children = append(n.Children, plac)
		}
	}
	add(n, "NOTE", e.notes)
	if ex.opt.Version7 {
		for _, p := range ex.parts[e.id] {
			if ex.included[p.personID] {
				asso := pointer("ASSO", personXref(p.personID))
				asso.Children = append(asso.Children, roleNode(p.role))
				n.Children = append(n.Children, asso)
			}
		}
	}
	ex.status(n, e.status, e.statusReason)
	ex.citations(n, "event", e.id)
	ex.mediaRefs(n, "event", e.id)
	for _, k := range ex.kept(e.extra) {
		if k.Tag != "PHRASE" {
			n.Children = append(n.Children, k)
			continue
		}
		// A DATE.PHRASE the import could not use is kept without its DATE.
		if dn := n.First("DATE"); dn != nil && ex.opt.Version7 && dn.First("PHRASE") == nil {
			dn.Children = append(dn.Children, k)
		} else {
			add(n, "NOTE", "Date phrase: "+k.Value)
		}
	}
	// 5.5.1 marks an event that happened without details with "Y".
	if n.Value == "" && len(n.Children) == 0 && !ex.opt.Version7 {
		n.Value = "Y"
	}
	return n
}

var roles7 = map[string]string{
	"child": "CHIL", "clergy": "CLERGY", "parent": "PARENT", "friend": "FRIEND", "godparent": "GODP",
	"spouse": "SPOU", "neighbor": "NGHBR", "officiant": "OFFICIATOR", "witness": "WITN",
}

func roleNode(role string) *gedcom.Node {
	if r, ok := roles7[role]; ok {
		return node("ROLE", r)
	}
	return node("ROLE", "OTHER", node("PHRASE", role))
}

func coord(v float64, pos, neg string) string {
	prefix := pos
	if v < 0 {
		prefix, v = neg, -v
	}
	return prefix + strconv.FormatFloat(v, 'f', -1, 64)
}

// date writes a DATE line: valid dates in the version's syntax, anything
// else as a phrase so it is not lost.
func (ex *exporter) date(n *gedcom.Node, raw string) {
	if raw == "" {
		return
	}
	d, err := gendate.Parse(raw)
	if ex.opt.Version7 {
		value, phrase := "", raw
		if err == nil {
			value, phrase = d.GEDCOM7()
		}
		dn := node("DATE", value)
		add(dn, "PHRASE", phrase)
		n.Children = append(n.Children, dn)
		return
	}
	if err != nil {
		n.Children = append(n.Children, node("DATE", "("+strings.Trim(raw, "()")+")"))
		return
	}
	n.Children = append(n.Children, node("DATE", d.String()))
}

func (ex *exporter) status(n *gedcom.Node, status, reason string) {
	if status == "" || status == StatusAccepted {
		return
	}
	st := node("_STAT", status)
	add(st, "NOTE", reason)
	n.Children = append(n.Children, st)
}

func (ex *exporter) citations(n *gedcom.Node, entity string, id int64) {
	for _, c := range ex.cites[fmt.Sprintf("%s:%d", entity, id)] {
		s := pointer("SOUR", fmt.Sprintf("S%d", c.sourceID))
		add(s, "PAGE", c.page)
		if c.text != "" {
			s.Children = append(s.Children, node("DATA", "", node("TEXT", c.text)))
		}
		if c.quality.Valid {
			s.Children = append(s.Children, node("QUAY", strconv.FormatInt(c.quality.Int64, 10)))
		}
		add(s, "NOTE", c.notes)
		s.Children = append(s.Children, ex.kept(c.extra)...)
		n.Children = append(n.Children, s)
		ex.out.Counts["citations"]++
	}
}

func (ex *exporter) mediaRefs(n *gedcom.Node, entity string, id int64) {
	for _, m := range ex.media[fmt.Sprintf("%s:%d", entity, id)] {
		n.Children = append(n.Children, pointer("OBJE", fmt.Sprintf("O%d", m)))
	}
}

// kept turns lines stored verbatim at import back into nodes, pointing
// their references at the exported records. A reference to something no
// longer exported is left out.
func (ex *exporter) kept(extra string) []*gedcom.Node {
	if extra == "" || extra == "{}" {
		return nil
	}
	var stored struct {
		Gedcom []keptNode `json:"gedcom"`
	}
	if err := json.Unmarshal([]byte(extra), &stored); err != nil {
		return nil
	}
	var out []*gedcom.Node
	for _, k := range stored.Gedcom {
		if n := ex.fromKept(k); n != nil {
			out = append(out, n)
		}
	}
	return out
}

func (ex *exporter) fromKept(k keptNode) *gedcom.Node {
	n := &gedcom.Node{Tag: k.Tag, Value: k.Value}
	if k.Pointer != "" {
		mapped, ok := ex.xrefMap[k.Pointer]
		if !ok && k.Pointer != "VOID" {
			ex.dangling++
			return nil
		}
		n.Pointer = mapped
		if k.Pointer == "VOID" {
			n.Pointer = "VOID"
		}
		if strings.HasPrefix(mapped, "I") {
			var id int64
			fmt.Sscanf(mapped, "I%d", &id)
			if !ex.included[id] {
				return nil
			}
		}
	}
	for _, c := range k.Children {
		if cn := ex.fromKept(c); cn != nil {
			n.Children = append(n.Children, cn)
		}
	}
	return n
}

var relationNames = map[string]string{
	"birth": "Natural", "adopted": "Adopted", "foster": "Foster", "step": "Step", "surrogate": "Surrogate",
	"sealing": "Sealing", "unknown": "Unknown",
}

func (ex *exporter) familyRecord(f exportFamily, events []exportEvent) *gedcom.Node {
	r := &gedcom.Node{Xref: familyXref(f.id), Tag: "FAM"}
	if f.p1.Valid && ex.included[f.p1.Int64] {
		r.Children = append(r.Children, pointer("HUSB", personXref(f.p1.Int64)))
	}
	if f.p2.Valid && ex.included[f.p2.Int64] {
		r.Children = append(r.Children, pointer("WIFE", personXref(f.p2.Int64)))
	}
	for _, c := range f.children {
		if !ex.included[c.personID] {
			continue
		}
		chil := pointer("CHIL", personXref(c.personID))
		if c.rel1 != c.rel2 || pedigree(c.rel1, c.rel2, false) == "" && c.rel1 != "birth" {
			chil.Children = append(chil.Children, node("_FREL", relationNames[c.rel1]), node("_MREL", relationNames[c.rel2]))
		}
		r.Children = append(r.Children, chil)
	}
	if len(r.Children) == 0 {
		// Everyone in it was left out for privacy.
		return nil
	}
	hidden := (f.p1.Valid && ex.nameOnly[f.p1.Int64]) || (f.p2.Valid && ex.nameOnly[f.p2.Int64])
	if !hidden {
		for _, e := range events {
			r.Children = append(r.Children, ex.eventNode(e))
		}
		add(r, "NOTE", f.notes)
		ex.citations(r, "family", f.id)
		ex.mediaRefs(r, "family", f.id)
		r.Children = append(r.Children, ex.kept(f.extra)...)
	}
	return r
}

func (ex *exporter) sourceRecords(ctx context.Context) ([]*gedcom.Node, error) {
	rows, err := ex.s.DB.QueryContext(ctx, `
		SELECT id, title, author, publication, call_number, notes, repository_id, extra_json FROM sources WHERE tree_id = ? ORDER BY id`, ex.a.TreeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*gedcom.Node
	for rows.Next() {
		var id int64
		var title, author, publ, call, notes, extra string
		var repo sql.NullInt64
		if err := rows.Scan(&id, &title, &author, &publ, &call, &notes, &repo, &extra); err != nil {
			return nil, err
		}
		r := &gedcom.Node{Xref: fmt.Sprintf("S%d", id), Tag: "SOUR"}
		add(r, "TITL", title)
		add(r, "AUTH", author)
		add(r, "PUBL", publ)
		if repo.Valid {
			rn := pointer("REPO", fmt.Sprintf("R%d", repo.Int64))
			add(rn, "CALN", call)
			r.Children = append(r.Children, rn)
		}
		add(r, "NOTE", notes)
		ex.mediaRefs(r, "source", id)
		r.Children = append(r.Children, ex.kept(extra)...)
		out = append(out, r)
		ex.out.Counts["sources"]++
	}
	return out, rows.Err()
}

func (ex *exporter) repositoryRecords(ctx context.Context) ([]*gedcom.Node, error) {
	rows, err := ex.s.DB.QueryContext(ctx, `
		SELECT id, name, address, url, notes, extra_json FROM repositories WHERE tree_id = ? ORDER BY id`, ex.a.TreeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*gedcom.Node
	for rows.Next() {
		var id int64
		var name, address, url, notes, extra string
		if err := rows.Scan(&id, &name, &address, &url, &notes, &extra); err != nil {
			return nil, err
		}
		r := &gedcom.Node{Xref: fmt.Sprintf("R%d", id), Tag: "REPO"}
		add(r, "NAME", name)
		add(r, "ADDR", address)
		add(r, "WWW", url)
		add(r, "NOTE", notes)
		r.Children = append(r.Children, ex.kept(extra)...)
		out = append(out, r)
		ex.out.Counts["repositories"]++
	}
	return out, rows.Err()
}

var mediaExt = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp", "application/pdf": ".pdf",
	"audio/mpeg": ".mp3", "audio/mp4": ".m4a", "audio/ogg": ".ogg", "audio/wav": ".wav", "audio/webm": ".weba",
	"video/mp4": ".mp4", "video/webm": ".webm",
}

func (ex *exporter) mediaRecords(ctx context.Context) ([]*gedcom.Node, error) {
	rows, err := ex.s.DB.QueryContext(ctx, `
		SELECT id, sha256, mime, kind, title FROM media WHERE tree_id = ? ORDER BY id`, ex.a.TreeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*gedcom.Node
	for rows.Next() {
		var id int64
		var sha, mime, kind, title string
		if err := rows.Scan(&id, &sha, &mime, &kind, &title); err != nil {
			return nil, err
		}
		path := "media/" + sha + mediaExt[mime]
		file := node("FILE", path)
		if ex.opt.Version7 {
			form := node("FORM", mime)
			if kind == "image" {
				form.Children = append(form.Children, node("MEDI", "PHOTO"))
			}
			file.Children = append(file.Children, form)
			add(file, "TITL", title)
		} else {
			form := node("FORM", strings.TrimPrefix(mediaExt[mime], "."))
			if kind == "image" {
				form.Children = append(form.Children, node("TYPE", "photo"))
			}
			file.Children = append(file.Children, form)
			add(file, "TITL", title)
		}
		out = append(out, &gedcom.Node{Xref: fmt.Sprintf("O%d", id), Tag: "OBJE", Children: []*gedcom.Node{file}})
		ex.out.Media = append(ex.out.Media, MediaFile{SHA256: sha, Path: path})
		ex.out.Counts["media"]++
	}
	return out, rows.Err()
}

// livingAll computes the living state of every person in the tree with the
// same rules as personRefs, without an id list.
func (s *Store) livingAll(ctx context.Context, a Actor) (map[int64]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id, p.is_living,
			(SELECT e.date_sort FROM events e
				WHERE e.person_id = p.id AND e.type IN ('BIRT', 'CHR', 'BAPM') AND e.status <> 'disproven' AND e.date_sort IS NOT NULL
				ORDER BY e.type <> 'BIRT', e.date_sort LIMIT 1),
			EXISTS (SELECT 1 FROM events e
				WHERE e.person_id = p.id AND e.type IN ('DEAT', 'BURI', 'CREM') AND e.status <> 'disproven')
		FROM persons p WHERE p.tree_id = ?`, a.TreeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	thisYear := s.Now().Year()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		var stated, birthSort sql.NullInt64
		var dead bool
		if err := rows.Scan(&id, &stated, &birthSort, &dead); err != nil {
			return nil, err
		}
		switch {
		case stated.Valid:
			out[id] = stated.Int64 == 1
		case dead:
			out[id] = false
		case birthSort.Valid:
			out[id] = int(birthSort.Int64/10000) > thisYear-livingCutoffYears
		default:
			out[id] = true
		}
	}
	return out, rows.Err()
}

// heirloomRecords writes heirlooms as _HEIRLOOM records (see
// docs/gedcom-extensions.md). A holder left out for privacy keeps the
// custody entry without the pointer.
func (ex *exporter) heirloomRecords(ctx context.Context) ([]*gedcom.Node, error) {
	rows, err := ex.s.DB.QueryContext(ctx, `
		SELECT id, name, kind, description, made_date_raw, origin_place_id, current_location, notes
		FROM heirlooms WHERE tree_id = ? ORDER BY id`, ex.a.TreeID)
	if err != nil {
		return nil, err
	}
	type heirloom struct {
		id                                 int64
		name, kind, desc, made, loc, notes string
		place                              sql.NullInt64
	}
	var list []heirloom
	for rows.Next() {
		var h heirloom
		if err := rows.Scan(&h.id, &h.name, &h.kind, &h.desc, &h.made, &h.place, &h.loc, &h.notes); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []*gedcom.Node
	for _, h := range list {
		r := &gedcom.Node{Xref: fmt.Sprintf("H%d", h.id), Tag: "_HEIRLOOM"}
		add(r, "NAME", h.name)
		add(r, "TYPE", h.kind)
		add(r, "_DESC", h.desc)
		ex.date(r, h.made)
		if h.place.Valid {
			if pi, ok := ex.places[h.place.Int64]; ok {
				add(r, "PLAC", pi.fullName)
			}
		}
		add(r, "_LOC", h.loc)
		add(r, "NOTE", h.notes)
		crow, err := ex.s.DB.QueryContext(ctx, `
			SELECT person_id, from_date_raw, to_date_raw, how, notes FROM heirloom_custody WHERE heirloom_id = ? ORDER BY sort_order, id`, h.id)
		if err != nil {
			return nil, err
		}
		for crow.Next() {
			var pid sql.NullInt64
			var from, to, how, notes string
			if err := crow.Scan(&pid, &from, &to, &how, &notes); err != nil {
				crow.Close()
				return nil, err
			}
			c := node("_CUST", "")
			if pid.Valid && ex.included[pid.Int64] {
				c = pointer("_CUST", personXref(pid.Int64))
			}
			add(c, "_FROM", from)
			add(c, "_TO", to)
			add(c, "TYPE", how)
			add(c, "NOTE", notes)
			r.Children = append(r.Children, c)
		}
		crow.Close()
		ex.citations(r, "heirloom", h.id)
		ex.mediaRefs(r, "heirloom", h.id)
		out = append(out, r)
		ex.out.Counts["heirlooms"]++
	}
	return out, nil
}
