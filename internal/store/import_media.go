package store

import (
	"context"
	"strings"

	"github.com/praetorianer777/gotree/internal/gedcom"
)

// ImportFiles are the files that came with a GEDCOM upload (a GEDZIP). The
// API has already stored them and checked their type and size; the importer
// only records them.
type ImportFiles struct {
	// Media holds the stored files by the FILE value exactly as the
	// document writes it.
	Media map[string]NewMedia
	// Skipped holds, by FILE value, why a file was not taken.
	Skipped map[string]string
}

const (
	noFilesInUpload = "the file is not in the upload; import a GEDZIP or upload the file in GoTree"
	webFileReason   = "links to files on the web are not imported"
)

// missingReason says why a FILE value has no file: a web address is never
// imported, anything else is not in the upload.
func missingReason(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, scheme := range []string{"http://", "https://", "ftp://"} {
		if strings.HasPrefix(lower, scheme) {
			return webFileReason
		}
	}
	return noFilesInUpload
}

// objeFate is what happens to an OBJE line under a record.
type objeFate int

const (
	// objeKeep: the line is kept as written (the default of other()).
	objeKeep objeFate = iota
	// objeImported: it yields media, which linkMedia attaches.
	objeImported
	// objeInline: it points to a media object that has no file here; the
	// object is kept as an inline copy, so that what it says survives.
	objeInline
)

func (im *importer) objeFate(c *gedcom.Node) objeFate {
	if c.Pointer != "" {
		ids, ok := im.mediaByXref[c.Pointer]
		switch {
		case ok && len(ids) > 0:
			return objeImported
		case ok:
			return objeInline
		}
		return objeKeep
	}
	for _, f := range c.All("FILE") {
		if _, ok := im.files.Media[strings.TrimSpace(f.Value)]; ok {
			return objeImported
		}
	}
	return objeKeep
}

// reportMissingFiles reports why the files of an inline OBJE line were not
// imported, for a line that is kept as written.
func (im *importer) reportMissingFiles(path string, c *gedcom.Node) {
	for _, f := range c.All("FILE") {
		value := strings.TrimSpace(f.Value)
		reason, ok := im.files.Skipped[value]
		if !ok {
			reason = missingReason(value)
		}
		im.tag(path+".FILE", TagDropped, reason)
	}
}

// inlineObje is the media object that c points to, written out in place of
// the pointer, with the lines of the pointer added.
func (im *importer) inlineObje(c *gedcom.Node) *gedcom.Node {
	rec, _ := im.doc.RecordByXref(c.Pointer)
	cp := &gedcom.Node{Tag: "OBJE", Line: c.Line}
	cp.Children = append(cp.Children, rec.Children...)
	cp.Children = append(cp.Children, c.Children...)
	return cp
}

// importedMedia is what the link pass needs to know about a medium.
type importedMedia struct {
	kind string
	// title is the default title taken from the file name; a title from the
	// GEDCOM file replaces it, but never one that someone has edited.
	title string
}

// objeResult is what one OBJE node produced.
type objeResult struct {
	ids []int64
	// primary is a _PRIM Y line: the photo is the person's main one.
	primary bool
}

// checkImportMode applies the rules of an import that do not depend on
// the data: a valid mode and enough rights.
func checkImportMode(a Actor, mode string) error {
	if mode != ImportIntoEmpty && mode != ImportReplace {
		return &ValidationError{Fields: map[string]string{"mode": "must be empty or replace"}}
	}
	// The API checks this too; the store enforces it for every caller.
	if !a.CanEdit() {
		return ErrViewOnly
	}
	if mode == ImportReplace && a.Role != RoleOwner {
		return ErrOwnerOnly
	}
	return nil
}

// checkTreeEmpty returns ErrTreeNotEmpty when the tree has people, families
// or sources.
func checkTreeEmpty(ctx context.Context, q queryer, treeID int64) error {
	var n int
	if err := q.QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM persons WHERE tree_id = ?) + (SELECT count(*) FROM families WHERE tree_id = ?)
			+ (SELECT count(*) FROM sources WHERE tree_id = ?)`, treeID, treeID, treeID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrTreeNotEmpty
	}
	return nil
}

// CheckImportTarget answers, without changing anything, whether an import
// in this mode may start: it returns what ImportGEDCOM would return for the
// mode, the rights and a tree that is not empty. The API calls it before it
// stores the files of a zip, so that a refused import leaves none behind.
func (s *Store) CheckImportTarget(ctx context.Context, a Actor, mode string) error {
	if err := checkImportMode(a, mode); err != nil {
		return err
	}
	if mode == ImportIntoEmpty {
		return checkTreeEmpty(ctx, s.DB, a.TreeID)
	}
	return nil
}

// objeLink is one OBJE line that attaches media to a record.
type objeLink struct {
	// path is the report path of the owner, e.g. "INDI" or "INDI.BIRT".
	path string
	// entity is the media_links entity type of the owner.
	entity string
	// xref is the owner's record xref, for entities that are records.
	xref string
	// event is the event line, for entity "event".
	event *gedcom.Node
	obje  *gedcom.Node
}

// objeLinks lists the OBJE lines that the link pass reads: those of people,
// families, sources and heirlooms, and of their events and attributes. An
// OBJE anywhere else (under a citation, a name) is kept as written.
func objeLinks(doc *gedcom.Document) []objeLink {
	var out []objeLink
	for _, r := range doc.Records {
		var entity string
		var isEvent func(tag string) bool
		switch r.Tag {
		case "INDI":
			entity = "person"
			isEvent = func(tag string) bool { return personEventTags[tag] || attributeTags[tag] }
		case "FAM":
			entity = "family"
			isEvent = func(tag string) bool { return familyEventTagsIn[tag] }
		case "SOUR":
			entity = "source"
		case "_HEIRLOOM":
			entity = "heirloom"
		default:
			continue
		}
		for _, c := range r.Children {
			switch {
			case c.Tag == "OBJE":
				out = append(out, objeLink{path: r.Tag, entity: entity, xref: r.Xref, obje: c})
			case isEvent != nil && isEvent(c.Tag):
				for _, oc := range c.All("OBJE") {
					out = append(out, objeLink{path: r.Tag + "." + c.Tag, entity: "event", event: c, obje: oc})
				}
			}
		}
	}
	return out
}

// objeHandled reports whether the OBJE lines under a path are read by the
// link pass instead of being kept verbatim. It matches objeLinks.
func objeHandled(path string) bool {
	switch path {
	case "INDI", "FAM", "SOUR", "_HEIRLOOM":
		return true
	}
	owner, tag, ok := strings.Cut(path, ".")
	if !ok || strings.Contains(tag, ".") {
		return false
	}
	switch owner {
	case "INDI":
		return personEventTags[tag] || attributeTags[tag]
	case "FAM":
		return familyEventTagsIn[tag]
	}
	return false
}

// MediaPaths lists the FILE values of the media that an import reads from
// the document: those of the OBJE records and of the OBJE lines that the
// importer links to a record. They are distinct and in document order. The
// API looks them up in the zip before the import starts.
func MediaPaths(doc *gedcom.Document) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n *gedcom.Node) {
		for _, f := range n.All("FILE") {
			if v := strings.TrimSpace(f.Value); v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	for _, r := range doc.Records {
		if r.Tag == "OBJE" {
			add(r)
		}
	}
	for _, l := range objeLinks(doc) {
		if l.obje.Pointer == "" {
			add(l.obje)
		}
	}
	return out
}

// mediaRecords imports the OBJE records. Their media are linked to the
// records that point to them later, by linkMedia.
func (im *importer) mediaRecords(ctx context.Context, recs []*gedcom.Node) error {
	for _, r := range recs {
		res, err := im.objeMedia(ctx, "OBJE", r)
		if err != nil {
			return err
		}
		if r.Xref != "" {
			im.mediaByXref[r.Xref] = res.ids
		}
		if len(res.ids) > 0 {
			im.mapped("OBJE")
			continue
		}
		reason := ""
		if f := r.First("FILE"); f == nil {
			reason = "the media object has no file"
		} else if why, ok := im.files.Skipped[strings.TrimSpace(f.Value)]; ok {
			reason = why
		} else {
			reason = missingReason(f.Value)
		}
		im.tag("OBJE", TagDropped, reason)
	}
	return nil
}

// objeMedia reads one OBJE node, a record or an inline line, and records
// the media of its files. It returns their ids, including the ids of media
// that the tree already had.
func (im *importer) objeMedia(ctx context.Context, path string, n *gedcom.Node) (objeResult, error) {
	var res objeResult
	var kept []keptNode
	var notes []string
	var objTitle string
	for _, c := range n.Children {
		switch c.Tag {
		case "FILE":
			value := strings.TrimSpace(c.Value)
			nm, ok := im.files.Media[value]
			if !ok {
				reason, skipped := im.files.Skipped[value]
				if !skipped {
					reason = missingReason(value)
				}
				im.tag(path+".FILE", TagDropped, reason)
				continue
			}
			id, err := im.addMedia(ctx, nm)
			if err != nil {
				return res, err
			}
			res.ids = append(res.ids, id)
			im.mapped(path + ".FILE")
			for _, fc := range c.Children {
				switch fc.Tag {
				case "TITL":
					if err := im.mediaTitle(ctx, id, fc.Value); err != nil {
						return res, err
					}
					im.mapped(path + ".FILE.TITL")
				case "FORM":
					// The type is detected from the file, not trusted from the tag.
					im.mapped(path + ".FILE.FORM")
					for _, sub := range fc.Children {
						if sub.Tag == "TYPE" || sub.Tag == "MEDI" {
							im.mapped(path + ".FILE.FORM." + sub.Tag)
						} else {
							im.tag(path+".FILE.FORM."+sub.Tag, TagDropped, "not kept on a media file")
						}
					}
				default:
					im.tag(path+".FILE."+fc.Tag, TagDropped, "not kept on a media file")
				}
			}
		case "TITL": // GEDCOM 5.5
			objTitle = c.Value
			im.mapped(path + ".TITL")
		case "FORM": // GEDCOM 5.5
			im.mapped(path + ".FORM")
		case "_PRIM":
			res.primary = strings.EqualFold(strings.TrimSpace(c.Value), "Y")
			im.mapped(path + "._PRIM")
		case "NOTE", "SNOTE":
			notes = append(notes, im.noteText(c))
			im.mapped(path + "." + c.Tag)
		case "BLOB":
			im.tag(path+".BLOB", TagDropped, "embedded BLOB data is not read")
		default:
			im.other(path, c, &kept)
		}
	}
	if len(res.ids) == 0 {
		return res, nil
	}
	for _, id := range res.ids {
		if err := im.mediaTitle(ctx, id, objTitle); err != nil {
			return res, err
		}
	}
	first := res.ids[0]
	if desc := joinNotes(notes); desc != "" {
		if _, err := im.tx.ExecContext(ctx, `UPDATE media SET description = ? WHERE id = ? AND description = ''`, desc, first); err != nil {
			return res, err
		}
	}
	if len(kept) > 0 {
		if _, err := im.tx.ExecContext(ctx, `UPDATE media SET extra_json = ? WHERE id = ? AND extra_json = '{}'`, extraJSON(kept), first); err != nil {
			return res, err
		}
	}
	if n.Xref != "" {
		if _, err := im.tx.ExecContext(ctx, `UPDATE media SET gedcom_xref = ? WHERE id = ? AND gedcom_xref = ''`, n.Xref, first); err != nil {
			return res, err
		}
	}
	return res, nil
}

// addMedia records one stored file, or finds the medium the tree already
// has for it.
func (im *importer) addMedia(ctx context.Context, nm NewMedia) (int64, error) {
	id, created, err := im.s.insertMedia(ctx, im.tx, im.a, nm, im.now)
	if err != nil {
		return 0, err
	}
	if created {
		im.report.Counts["media"]++
	}
	im.mediaInfo[id] = importedMedia{
		kind:  nm.Kind,
		title: strings.TrimSuffix(nm.OriginalName, extOf(nm.OriginalName)),
	}
	return id, nil
}

// mediaTitle sets the title of a medium, unless it already has one that
// is not just the file name.
func (im *importer) mediaTitle(ctx context.Context, id int64, title string) error {
	title = truncate(collapse(title), 300)
	if title == "" {
		return nil
	}
	_, err := im.tx.ExecContext(ctx, `UPDATE media SET title = ? WHERE id = ? AND (title = '' OR title = ?)`,
		title, id, im.mediaInfo[id].title)
	return err
}

// linkMedia attaches the media to the records whose OBJE lines point to
// them or contain them. It runs once all records exist.
func (im *importer) linkMedia(ctx context.Context) error {
	for _, l := range objeLinks(im.doc) {
		if im.objeFate(l.obje) != objeImported {
			continue // kept with its owner by other()
		}
		var id int64
		var ok bool
		switch l.entity {
		case "person":
			id, ok = im.persons[l.xref]
		case "family":
			id, ok = im.families[l.xref]
		case "source":
			id, ok = im.sources[l.xref]
		case "heirloom":
			id, ok = im.heirlooms[l.xref]
		case "event":
			id, ok = im.eventNodes[l.event]
		}
		if !ok {
			im.tag(l.path+".OBJE", TagDropped, "the record it belongs to was not imported")
			continue
		}
		if err := im.linkOne(ctx, l, id); err != nil {
			return err
		}
	}
	return nil
}

// linkOne attaches the media of one OBJE line to the record with this id.
func (im *importer) linkOne(ctx context.Context, l objeLink, owner int64) error {
	c := l.obje
	path := l.path + ".OBJE"
	var ids []int64
	primary := false
	if c.Pointer != "" {
		var ok bool
		if ids, ok = im.mediaByXref[c.Pointer]; !ok {
			if c.Pointer == "VOID" {
				im.tag(path, TagDropped, "it points to no media object")
			} else {
				im.broken(c, "the media object")
			}
			return nil
		}
		if len(ids) == 0 {
			im.tag(path, TagDropped, "the media file was not imported")
			return nil
		}
		for _, sub := range c.Children {
			switch sub.Tag {
			case "TITL":
				for _, id := range ids {
					if err := im.mediaTitle(ctx, id, sub.Value); err != nil {
						return err
					}
				}
				im.mapped(path + ".TITL")
			case "_PRIM":
				primary = strings.EqualFold(strings.TrimSpace(sub.Value), "Y")
				im.mapped(path + "._PRIM")
			case "CROP":
				im.tag(path+".CROP", TagDropped, "cropping a linked photo is not kept")
			default:
				im.tag(path+"."+sub.Tag, TagDropped, "not kept on a media link")
			}
		}
	} else {
		res, err := im.objeMedia(ctx, path, c)
		if err != nil {
			return err
		}
		if len(res.ids) == 0 {
			im.tag(path, TagDropped, "the media file was not imported")
			return nil
		}
		ids, primary = res.ids, res.primary
	}
	for _, id := range ids {
		if err := insertMediaLink(ctx, im.tx, id, MediaLinkInput{EntityType: l.entity, EntityID: owner}); err != nil {
			return err
		}
	}
	im.mapped(path)
	if primary && l.entity == "person" {
		for _, id := range ids {
			if im.mediaInfo[id].kind != "image" {
				continue
			}
			if _, err := im.tx.ExecContext(ctx, `UPDATE persons SET portrait_media_id = ? WHERE id = ? AND portrait_media_id IS NULL`, id, owner); err != nil {
				return err
			}
			break
		}
	}
	return nil
}
