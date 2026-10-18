package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/praetorianer777/gotree/internal/db"
	"github.com/praetorianer777/gotree/internal/gedcom"
	"github.com/praetorianer777/gotree/internal/gendate"
)

// Import modes.
const (
	ImportIntoEmpty = "empty"
	ImportReplace   = "replace"
)

// ErrTreeNotEmpty is returned when importing into a tree that has data
// without asking to replace it.
var ErrTreeNotEmpty = errors.New("the tree already has people, families or sources")

// Tag outcomes in the import report.
const (
	TagMapped  = "mapped"
	TagKept    = "kept"
	TagDropped = "dropped"
)

// TagStat says what happened to one kind of GEDCOM line.
type TagStat struct {
	// Path is the tag with its context, e.g. "INDI.BIRT.DATE".
	Path    string `json:"path"`
	Count   int    `json:"count"`
	Outcome string `json:"outcome"`
	// Reason is the English reason a line was dropped; ReasonCode is its
	// stable code (one of DropReasonCodes) that clients translate.
	Reason     string `json:"reason,omitempty"`
	ReasonCode string `json:"reasonCode,omitempty"`
}

// ImportReport describes what an import did.
type ImportReport struct {
	Version  string `json:"version"`
	Encoding string `json:"encoding"`
	Source   string `json:"source"`
	// Counts holds the created records by kind.
	Counts   map[string]int   `json:"counts"`
	Tags     []TagStat        `json:"tags"`
	Warnings []gedcom.Warning `json:"warnings"`
	// InvalidDates are dates kept as written because they are not valid
	// GEDCOM; Examples shows a few.
	InvalidDates        int              `json:"invalidDates"`
	InvalidDateExamples []gedcom.Warning `json:"invalidDateExamples"`
	BrokenReferences    []gedcom.Warning `json:"brokenReferences"`
	// ChildrenFromFAMC counts children that their family did not list under
	// CHIL but that named the family in their own FAMC; they were added to
	// it. The examples show a few.
	ChildrenFromFAMC         int              `json:"childrenFromFamc"`
	ChildrenFromFAMCExamples []gedcom.Warning `json:"childrenFromFamcExamples"`
	// ShareLinksKept counts working links to a branch whose person was
	// found again in the file (same GEDCOM ID and name); ShareLinksLost
	// are the labels of working links whose person was not, and which
	// were deleted with the old tree. Only a replacing import sets them.
	ShareLinksKept int      `json:"shareLinksKept"`
	ShareLinksLost []string `json:"shareLinksLost"`
	DurationMS     int64    `json:"durationMs"`
}

// keptNode is a GEDCOM line stored verbatim in extra_json, so an export can
// write it back.
type keptNode struct {
	Tag      string     `json:"t"`
	Value    string     `json:"v,omitempty"`
	Pointer  string     `json:"p,omitempty"`
	Children []keptNode `json:"c,omitempty"`
}

func toKept(n *gedcom.Node) keptNode {
	k := keptNode{Tag: n.Tag, Value: n.Value, Pointer: n.Pointer}
	for _, c := range n.Children {
		k.Children = append(k.Children, toKept(c))
	}
	return k
}

func extraJSON(kept []keptNode) string {
	if len(kept) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(map[string][]keptNode{"gedcom": kept})
	return string(b)
}

var (
	personEventTags = map[string]bool{
		"BIRT": true, "CHR": true, "DEAT": true, "BURI": true, "CREM": true, "ADOP": true, "BAPM": true,
		"BARM": true, "BASM": true, "BLES": true, "CHRA": true, "CONF": true, "FCOM": true, "ORDN": true,
		"NATU": true, "EMIG": true, "IMMI": true, "CENS": true, "PROB": true, "WILL": true, "GRAD": true,
		"RETI": true, "EVEN": true, "_MILT": true, "_MIL": true,
	}
	attributeTags = map[string]bool{
		"OCCU": true, "EDUC": true, "RELI": true, "RESI": true, "TITL": true, "NATI": true, "CAST": true,
		"DSCR": true, "IDNO": true, "NCHI": true, "NMR": true, "PROP": true, "SSN": true, "FACT": true,
	}
	familyEventTagsIn = map[string]bool{
		"MARR": true, "DIV": true, "DIVF": true, "ENGA": true, "MARB": true, "MARC": true, "MARL": true,
		"MARS": true, "ANUL": true, "EVEN": true, "CENS": true, "RESI": true,
	}
	// Tags that GoTree replaces with its own data when exporting.
	droppedTags = map[string]string{
		"CHAN": DropOwnHistory,
		"RIN":  DropOldRecordNumber,
		"FAMS": DropFollowsFromFamilies,
	}
)

// Codes of the lines an import report adds (the parser's are in
// gedcom.WarningCodes). Clients translate them with the line's params.
const (
	ReportBrokenReference  = "broken_reference"
	ReportPartnerAndChild  = "partner_and_child"
	ReportFamcChild        = "famc_child"
	ReportFamcChildUnnamed = "famc_child_unnamed"
	ReportInvalidDate      = "invalid_date"
)

// ReportCodes lists every code an import report line can carry.
var ReportCodes = []string{ReportBrokenReference, ReportPartnerAndChild, ReportFamcChild, ReportFamcChildUnnamed, ReportInvalidDate}

// Codes of the reasons a line is dropped (TagStat.ReasonCode).
const (
	DropOwnHistory          = "own_history"
	DropOldRecordNumber     = "old_record_number"
	DropFollowsFromFamilies = "follows_from_families"
	DropOwnSubmitter        = "own_submitter"
	DropMediaRecords        = "media_records"
	DropUnknownRecord       = "unknown_record"
	DropPlaceForm           = "place_form"
	DropHeirloomDatePhrase  = "heirloom_date_phrase"
	DropHeirloomCustody     = "heirloom_custody_field"
	DropHeirloomField       = "heirloom_field"
	DropStatusOnlyReason    = "status_only_reason"
	DropNameStatus          = "name_status"
	DropWebLink             = "web_link"
	DropFileNotInUpload     = "file_not_in_upload"
	DropFileNotInZip        = "file_not_in_zip"
	DropFileTooLarge        = "file_too_large"
	DropZipTooLarge         = "zip_too_large"
	DropFileType            = "file_type_unsupported"
	DropImagePixels         = "image_too_large"
	DropFileUnreadable      = "file_unreadable"
	DropMediaNoFile         = "media_no_file"
	DropMarriedNameField    = "married_name_field"
	DropNameEmpty           = "name_empty"
	DropHeirloomMedia       = "heirloom_media"
	DropMediaFileField      = "media_file_field"
	DropBlob                = "blob_not_read"
	DropMediaRecordSkipped  = "media_record_not_imported"
	DropMediaNoObject       = "media_no_object"
	DropMediaFileMissing    = "media_file_not_imported"
	DropMediaCrop           = "media_crop"
	DropMediaLinkField      = "media_link_field"
	DropAssoSources         = "asso_sources"
)

// dropReasonText is the English reason of each drop code; TagStat.Reason is
// the fallback for clients that do not know a code.
var dropReasonText = map[string]string{
	DropOwnHistory:          "GoTree keeps its own change history",
	DropOldRecordNumber:     "a record number of the old program",
	DropFollowsFromFamilies: "follows from the families",
	DropOwnSubmitter:        "GoTree writes its own submitter",
	DropMediaRecords:        "media files are not part of a GEDCOM file; upload them in GoTree",
	DropUnknownRecord:       "unknown record type",
	DropPlaceForm:           "the place hierarchy describes the levels",
	DropHeirloomDatePhrase:  "heirlooms have nowhere to keep it",
	DropHeirloomCustody:     "not part of an heirloom's custody in GoTree",
	DropHeirloomField:       "not part of an heirloom in GoTree",
	DropStatusOnlyReason:    "a status has only a reason",
	DropNameStatus:          "the main name has no status in GoTree",
	DropWebLink:             "links to files on the web are not imported",
	DropFileNotInUpload:     "the file is not in the upload; import a GEDZIP or upload the file in GoTree",
	DropFileNotInZip:        "the file is not in the zip",
	DropFileTooLarge:        "the file is larger than the upload limit",
	DropZipTooLarge:         "the zip unpacks to more than the import limit",
	DropFileType:            "this kind of file is not supported",
	DropImagePixels:         "the image is larger than the pixel limit",
	DropFileUnreadable:      "the file in the zip cannot be read",
	DropMediaNoFile:         "the media object has no file",
	DropMarriedNameField:    "not kept for a married name",
	DropNameEmpty:           "the name is empty",
	DropHeirloomMedia:       "heirlooms have nowhere to keep a media line without a file",
	DropMediaFileField:      "not kept on a media file",
	DropBlob:                "embedded BLOB data is not read",
	DropMediaRecordSkipped:  "the record it belongs to was not imported",
	DropMediaNoObject:       "it points to no media object",
	DropMediaFileMissing:    "the media file was not imported",
	DropMediaCrop:           "cropping a linked photo is not kept",
	DropMediaLinkField:      "not kept on a media link",
	DropAssoSources:         "sources of a participant are not kept",
}

// DropReasonCodes lists every drop code.
var DropReasonCodes = []string{
	DropOwnHistory, DropOldRecordNumber, DropFollowsFromFamilies, DropOwnSubmitter, DropMediaRecords,
	DropUnknownRecord, DropPlaceForm, DropHeirloomDatePhrase, DropHeirloomCustody, DropHeirloomField,
	DropStatusOnlyReason, DropNameStatus,
	DropWebLink, DropFileNotInUpload, DropFileNotInZip, DropFileTooLarge, DropZipTooLarge, DropFileType, DropImagePixels, DropFileUnreadable, DropMediaNoFile, DropMarriedNameField, DropNameEmpty, DropHeirloomMedia, DropMediaFileField, DropBlob, DropMediaRecordSkipped, DropMediaNoObject, DropMediaFileMissing, DropMediaCrop, DropMediaLinkField, DropAssoSources,
}

// What a broken reference points at: the keys of broken's "what" param
// (transfer.refKind in the locale files) and their English words.
const (
	RefAssociatedPerson = "associatedPerson"
	RefNote             = "note"
	RefRepository       = "repository"
	RefSource           = "source"
	RefFamily           = "family"
	RefPartner          = "partner"
	RefChild            = "child"
	RefHolder           = "holder"
	RefFamilyEvent      = "familyEvent"
)

var refKindText = map[string]string{
	RefAssociatedPerson: "the associated person",
	RefNote:             "the note",
	RefRepository:       "the repository",
	RefSource:           "the source",
	RefFamily:           "the family",
	RefPartner:          "the partner",
	RefChild:            "the child",
	RefHolder:           "the holder",
	RefFamilyEvent:      "the family event",
}

// RefKinds lists every key broken accepts.
var RefKinds = []string{RefAssociatedPerson, RefNote, RefRepository, RefSource, RefFamily, RefPartner, RefChild, RefHolder, RefFamilyEvent}

type importer struct {
	s      *Store
	tx     *sql.Tx
	a      Actor
	doc    *gedcom.Document
	now    string
	report *ImportReport
	tags   map[string]*TagStat

	persons  map[string]int64
	families map[string]int64
	sources  map[string]int64
	repos    map[string]int64
	notes    map[string]string
	places   map[string]int64
	// pedigree[child][family] is the FAMC.PEDI relation of a child.
	pedigree map[string]map[string]string
	deferred []deferredParticipant
	// familyEvents[famXref][tag] are the ids of a family's events of one
	// type, in file order, for 5.5.1 ASSO lines that name a family event.
	familyEvents map[string]map[string][]int64
	// famcLinks are the children's own FAMC lines, applied after all
	// families are imported.
	famcLinks []famcLink
	// chilSeen holds famXref + "\x00" + childXref for every CHIL line.
	chilSeen map[string]bool
	// familyPartners are the partners of each family by its xref.
	familyPartners map[string][2]sql.NullInt64

	// Media of the upload (import_media.go): the files found for the FILE
	// values, the media of each OBJE record by its xref, and the events and
	// heirlooms by node and xref for the link pass.
	files       ImportFiles
	mediaByXref map[string][]int64
	mediaInfo   map[int64]importedMedia
	eventNodes  map[*gedcom.Node]int64
	heirlooms   map[string]int64

	insertPerson, insertEvent, insertName, insertFTS *sql.Stmt
}

type deferredParticipant struct {
	eventID int64
	xref    string
	role    string
	custom  string
	notes   string
	line    int
	// famXref, tag and ordinal name a family event (eventID is 0 until the
	// family's events are all imported).
	famXref string
	tag     string
	ordinal int
}

// famcLink is a child's own INDI.FAMC line; families that do not list
// the child under CHIL get it after all families are imported.
type famcLink struct {
	childID   int64
	childXref string
	family    string // the FAM xref
	pedi      string // FAMC.PEDI, "" when absent
	line      int
}

// tag counts one line of the given outcome. A dropped line carries the code
// of its reason (one of DropReasonCodes); other outcomes pass "".
// Reasons are part of the key: dropped lines of one kind may have
// different causes, and the report should name each.
func (im *importer) tag(path, outcome, reasonCode string) {
	key := path + "|" + outcome + "|" + reasonCode
	t := im.tags[key]
	if t == nil {
		t = &TagStat{Path: path, Outcome: outcome, Reason: dropReasonText[reasonCode], ReasonCode: reasonCode}
		im.tags[key] = t
	}
	t.Count++
}

func (im *importer) mapped(path string) { im.tag(path, TagMapped, "") }

func (im *importer) keep(path string, n *gedcom.Node, into *[]keptNode) {
	im.tag(path+"."+n.Tag, TagKept, "")
	*into = append(*into, toKept(n))
}

// broken notes a pointer to a record that is not in the file; what is one
// of RefKinds.
func (im *importer) broken(n *gedcom.Node, what string) {
	if len(im.report.BrokenReferences) < 50 {
		im.report.BrokenReferences = append(im.report.BrokenReferences, gedcom.Warning{
			Line: n.Line, Code: ReportBrokenReference,
			Params:  map[string]any{"what": what, "xref": n.Pointer},
			Message: fmt.Sprintf("%s @%s@ does not exist", refKindText[what], n.Pointer),
		})
	}
}

// partnerAndChild is the report line for a person listed as a child of a
// family they are a partner in; the child link is skipped.
func partnerAndChild(line int, xref, family string) gedcom.Warning {
	return gedcom.Warning{
		Line: line, Code: ReportPartnerAndChild,
		Params:  map[string]any{"xref": xref, "family": family},
		Message: fmt.Sprintf("@%s@ is both a partner and a child of family @%s@; the child link was skipped", xref, family),
	}
}

// ImportGEDCOM loads a parsed GEDCOM file into the actor's tree, in one
// transaction: either everything is imported or nothing. Media objects are
// reported as dropped, because the file has no files to go with them.
func (s *Store) ImportGEDCOM(ctx context.Context, a Actor, doc *gedcom.Document, mode string) (ImportReport, error) {
	return s.ImportGEDCOMFiles(ctx, a, doc, mode, ImportFiles{})
}

// ImportGEDCOMFiles is ImportGEDCOM for an upload that carries files (a
// GEDZIP), which the media objects of the file may refer to.
func (s *Store) ImportGEDCOMFiles(ctx context.Context, a Actor, doc *gedcom.Document, mode string, files ImportFiles) (ImportReport, error) {
	started := s.Now()
	report := ImportReport{
		Version: doc.Version, Encoding: doc.Encoding, Source: doc.Source,
		Counts: map[string]int{}, Warnings: doc.Warnings,
		InvalidDateExamples: []gedcom.Warning{}, BrokenReferences: []gedcom.Warning{},
		ChildrenFromFAMCExamples: []gedcom.Warning{}, ShareLinksLost: []string{},
	}
	if report.Warnings == nil {
		report.Warnings = []gedcom.Warning{}
	}
	if err := checkImportMode(a, mode); err != nil {
		return report, err
	}

	err := s.tx(ctx, func(tx *sql.Tx) error {
		var savedLinks []savedShareLink
		if mode == ImportReplace {
			var err error
			if savedLinks, err = saveShareLinks(ctx, tx, a.TreeID); err != nil {
				return err
			}
			if err := clearTree(ctx, tx, a.TreeID); err != nil {
				return err
			}
			if err := clearTranscriptionPeople(ctx, tx, a.TreeID); err != nil {
				return err
			}
		} else if err := checkTreeEmpty(ctx, tx, a.TreeID); err != nil {
			return err
		}

		im := &importer{
			s: s, tx: tx, a: a, doc: doc, now: s.now(), report: &report, tags: map[string]*TagStat{},
			persons: map[string]int64{}, families: map[string]int64{}, sources: map[string]int64{},
			repos: map[string]int64{}, notes: map[string]string{}, places: map[string]int64{},
			pedigree: map[string]map[string]string{},
			chilSeen: map[string]bool{}, familyEvents: map[string]map[string][]int64{}, familyPartners: map[string][2]sql.NullInt64{},
			files: files, mediaByXref: map[string][]int64{}, mediaInfo: map[int64]importedMedia{},
			eventNodes: map[*gedcom.Node]int64{}, heirlooms: map[string]int64{},
		}
		if err := im.prepare(ctx); err != nil {
			return err
		}
		defer im.close()
		if err := im.run(ctx); err != nil {
			return err
		}
		if err := restoreShareLinks(ctx, tx, a.TreeID, savedLinks, s.Now(), &report); err != nil {
			return err
		}
		summary, _ := json.Marshal(map[string]any{"source": doc.Source, "version": doc.Version, "counts": report.Counts})
		_, err := tx.ExecContext(ctx, `
			INSERT INTO change_log (tree_id, user_id, entity_type, entity_id, action, after_json, at)
			VALUES (?, ?, 'import', ?, 'create', ?, ?)`, a.TreeID, nullID(a.UserID), a.TreeID, string(summary), im.now)
		return err
	})
	report.DurationMS = s.Now().Sub(started).Milliseconds()
	return report, err
}

// clearTree removes the genealogy data of a tree. Media stay, without
// their links. Transcription drafts and share links to a branch are handled
// by ImportGEDCOM around it.
func clearTree(ctx context.Context, tx *sql.Tx, treeID int64) error {
	for _, q := range []string{
		`DELETE FROM persons_fts WHERE rowid IN (SELECT id FROM persons WHERE tree_id = ?)`,
		`DELETE FROM events WHERE tree_id = ?`,
		`DELETE FROM families WHERE tree_id = ?`,
		`UPDATE persons SET portrait_media_id = NULL, portrait_region_id = NULL WHERE tree_id = ?`,
		`DELETE FROM heirlooms WHERE tree_id = ?`,
		`DELETE FROM persons WHERE tree_id = ?`,
		`DELETE FROM citations WHERE tree_id = ?`,
		`DELETE FROM sources WHERE tree_id = ?`,
		`DELETE FROM repositories WHERE tree_id = ?`,
		// Places reference their parents (ON DELETE RESTRICT), so the links
		// go first.
		`UPDATE places SET parent_id = NULL WHERE tree_id = ?`,
		`DELETE FROM places WHERE tree_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, treeID); err != nil {
			return err
		}
	}
	return nil
}

func (im *importer) prepare(ctx context.Context) error {
	var err error
	if im.insertPerson, err = im.tx.PrepareContext(ctx, `
		INSERT INTO persons (tree_id, given_names, surname, name_prefix, name_suffix, nickname, sex, notes, gedcom_xref,
			extra_json, created_at, updated_at, created_by, updated_by, surname_key, given_key, name_notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
		return err
	}
	if im.insertEvent, err = im.tx.PrepareContext(ctx, `
		INSERT INTO events (tree_id, person_id, family_id, type, custom_label, date_raw, date, date_sort, date_sort_end,
			date_qualifier, place_id, description, notes, status, status_reason, sort_order, extra_json, created_at, updated_at,
			created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
		return err
	}
	if im.insertName, err = im.tx.PrepareContext(ctx, `
		INSERT INTO person_names (person_id, type, given_names, surname, name_prefix, name_suffix, nickname, status, status_reason, sort_order, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
		return err
	}
	im.insertFTS, err = im.tx.PrepareContext(ctx, `INSERT INTO persons_fts (rowid, names) VALUES (?, ?)`)
	return err
}

func (im *importer) close() {
	for _, st := range []*sql.Stmt{im.insertPerson, im.insertEvent, im.insertName, im.insertFTS} {
		if st != nil {
			st.Close()
		}
	}
}

func (im *importer) run(ctx context.Context) error {
	byTag := map[string][]*gedcom.Node{}
	for _, r := range im.doc.Records {
		byTag[r.Tag] = append(byTag[r.Tag], r)
	}

	for _, r := range append(byTag["NOTE"], byTag["SNOTE"]...) {
		if r.Xref != "" {
			im.notes[r.Xref] = r.Value
			im.mapped(r.Tag)
		}
	}
	if err := im.mediaRecords(ctx, byTag["OBJE"]); err != nil {
		return err
	}
	for _, r := range byTag["REPO"] {
		if err := im.repository(ctx, r); err != nil {
			return err
		}
	}
	for _, r := range byTag["SOUR"] {
		if err := im.source(ctx, r); err != nil {
			return err
		}
	}
	// Child relations are written on the child (FAMC.PEDI) but needed when
	// the family is imported.
	for _, r := range byTag["INDI"] {
		for _, famc := range r.All("FAMC") {
			if pedi := famc.Text("PEDI"); pedi != "" && famc.Pointer != "" {
				if im.pedigree[r.Xref] == nil {
					im.pedigree[r.Xref] = map[string]string{}
				}
				im.pedigree[r.Xref][famc.Pointer] = pedi
			}
		}
	}
	for _, r := range byTag["INDI"] {
		if err := im.person(ctx, r); err != nil {
			return err
		}
	}
	for _, r := range byTag["FAM"] {
		if err := im.family(ctx, r); err != nil {
			return err
		}
	}
	if err := im.linkFAMCChildren(ctx); err != nil {
		return err
	}
	for _, r := range byTag["_HEIRLOOM"] {
		if err := im.heirloom(ctx, r); err != nil {
			return err
		}
	}
	if err := im.linkMedia(ctx); err != nil {
		return err
	}
	for _, p := range im.deferred {
		if p.famXref != "" {
			ids := im.familyEvents[p.famXref][p.tag]
			if p.ordinal < 1 || p.ordinal > len(ids) {
				im.broken(&gedcom.Node{Line: p.line, Pointer: p.famXref}, RefFamilyEvent)
				continue
			}
			p.eventID = ids[p.ordinal-1]
		}
		pid, ok := im.persons[p.xref]
		if !ok {
			im.broken(&gedcom.Node{Line: p.line, Pointer: p.xref}, RefAssociatedPerson)
			continue
		}
		if _, err := im.tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO event_participants (event_id, person_id, role, custom_role, notes) VALUES (?, ?, ?, ?, ?)`, p.eventID, pid, p.role, p.custom, p.notes); err != nil {
			return err
		}
	}
	for tag, recs := range byTag {
		switch tag {
		case "INDI", "FAM", "SOUR", "REPO", "NOTE", "SNOTE", "_HEIRLOOM", "OBJE":
		case "SUBM", "SUBN":
			for range recs {
				im.tag(tag, TagDropped, DropOwnSubmitter)
			}
		default:
			for range recs {
				im.tag(tag, TagDropped, DropUnknownRecord)
			}
		}
	}

	stats := make([]TagStat, 0, len(im.tags))
	for _, t := range im.tags {
		stats = append(stats, *t)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Outcome != stats[j].Outcome {
			return stats[i].Outcome > stats[j].Outcome
		}
		return stats[i].Path < stats[j].Path
	})
	im.report.Tags = stats
	return nil
}

func (im *importer) noteText(n *gedcom.Node) string {
	if n.Pointer != "" {
		text, ok := im.notes[n.Pointer]
		if !ok {
			im.broken(n, RefNote)
		}
		return text
	}
	return n.Value
}

func joinNotes(notes []string) string {
	var kept []string
	for _, n := range notes {
		if n = strings.TrimSpace(n); n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "\n\n")
}

func (im *importer) repository(ctx context.Context, r *gedcom.Node) error {
	var kept []keptNode
	var notes []string
	name, address, url := "", "", ""
	for _, c := range r.Children {
		switch c.Tag {
		case "NAME":
			name = c.Value
			im.mapped("REPO.NAME")
		case "ADDR":
			address = flattenAddress(c)
			im.mapped("REPO.ADDR")
		case "WWW", "_URL", "URL":
			if url == "" {
				url = c.Value
				im.mapped("REPO." + c.Tag)
			} else {
				im.keep("REPO", c, &kept)
			}
		case "NOTE", "SNOTE":
			notes = append(notes, im.noteText(c))
			im.mapped("REPO." + c.Tag)
		default:
			im.other("REPO", c, &kept)
		}
	}
	if strings.TrimSpace(name) == "" {
		name = "Repository " + r.Xref
	}
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	res, err := im.tx.ExecContext(ctx, `
		INSERT INTO repositories (tree_id, name, address, url, notes, gedcom_xref, extra_json, created_at, updated_at, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		im.a.TreeID, name, address, url, joinNotes(notes), r.Xref, extraJSON(kept), im.now, im.now, nullID(im.a.UserID), nullID(im.a.UserID))
	if err != nil {
		return err
	}
	im.repos[r.Xref], _ = res.LastInsertId()
	im.report.Counts["repositories"]++
	im.mapped("REPO")
	return nil
}

func flattenAddress(n *gedcom.Node) string {
	parts := []string{n.Value}
	for _, tag := range []string{"ADR1", "ADR2", "ADR3", "POST", "CITY", "STAE", "CTRY"} {
		parts = append(parts, n.Text(tag))
	}
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" && !containsLine(out, p) {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n")
}

func containsLine(lines []string, s string) bool {
	for _, l := range lines {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// other files an unmapped line: known noise is dropped, everything else
// kept verbatim.
func (im *importer) other(ctx string, c *gedcom.Node, kept *[]keptNode) {
	if c.Tag == "OBJE" && objeHandled(ctx) {
		switch im.objeFate(c) {
		case objeImported:
			return // the link pass (linkMedia) reads it
		case objeInline:
			im.keep(ctx, im.inlineObje(c), kept)
			return
		}
		// The line is kept as written, but its files are not imported, and
		// the report says why.
		im.reportMissingFiles(ctx+".OBJE", c)
	}
	if code, ok := droppedTags[c.Tag]; ok {
		im.tag(ctx+"."+c.Tag, TagDropped, code)
		return
	}
	im.keep(ctx, c, kept)
}

// factStatus reads a GoTree _STAT line (docs/gedcom-extensions.md):
// the status and the reason in its NOTE. ok is false for a value GoTree
// does not know; the caller keeps such a line verbatim.
func (im *importer) factStatus(path string, c *gedcom.Node) (status, reason string, ok bool) {
	status = strings.ToLower(strings.TrimSpace(c.Value))
	if !validStatus(status) || (status == StatusAccepted && len(c.Children) > 0) {
		// GoTree never writes "accepted"; one with a note is another
		// program's, so keep it rather than drop the note.
		return "", "", false
	}
	var notes []string
	for _, sub := range c.Children {
		if sub.Tag == "NOTE" || sub.Tag == "SNOTE" {
			notes = append(notes, im.noteText(sub))
			im.mapped(path + "._STAT." + sub.Tag)
			continue
		}
		im.tag(path+"._STAT."+sub.Tag, TagDropped, DropStatusOnlyReason)
	}
	reason = joinNotes(notes)
	if reason == "" && status != StatusAccepted {
		reason = "?" // GoTree requires a reason; the file gave none
	}
	im.mapped(path + "._STAT")
	return status, reason, true
}

func (im *importer) source(ctx context.Context, r *gedcom.Node) error {
	var kept []keptNode
	var notes []string
	title, abbr, author, publ, callNumber := "", "", "", "", ""
	var repoID sql.NullInt64
	for _, c := range r.Children {
		switch c.Tag {
		case "TITL":
			title = c.Value
			im.mapped("SOUR.TITL")
		case "ABBR":
			abbr = c.Value
			im.mapped("SOUR.ABBR")
		case "AUTH":
			author = c.Value
			im.mapped("SOUR.AUTH")
		case "PUBL":
			publ = c.Value
			im.mapped("SOUR.PUBL")
		case "TEXT":
			notes = append(notes, c.Value)
			im.mapped("SOUR.TEXT")
		case "NOTE", "SNOTE":
			notes = append(notes, im.noteText(c))
			im.mapped("SOUR." + c.Tag)
		case "REPO":
			if id, ok := im.repos[c.Pointer]; ok && !repoID.Valid {
				repoID = sql.NullInt64{Int64: id, Valid: true}
				callNumber = c.Text("CALN")
				im.mapped("SOUR.REPO")
			} else if c.Pointer == "" && c.Value != "" {
				im.keep("SOUR", c, &kept)
			} else if !ok {
				im.broken(c, RefRepository)
			} else {
				im.keep("SOUR", c, &kept)
			}
		default:
			im.other("SOUR", c, &kept)
		}
	}
	if title == "" {
		title = abbr
	}
	if title == "" {
		title = "Source " + r.Xref
	}
	id, err := im.createSource(ctx, title, author, publ, callNumber, joinNotes(notes), repoID, r.Xref, kept)
	if err != nil {
		return err
	}
	im.sources[r.Xref] = id
	im.mapped("SOUR")
	return nil
}

func (im *importer) createSource(ctx context.Context, title, author, publ, call, notes string, repo sql.NullInt64, xref string, kept []keptNode) (int64, error) {
	res, err := im.tx.ExecContext(ctx, `
		INSERT INTO sources (tree_id, repository_id, call_number, title, author, publication, notes, gedcom_xref, extra_json,
			created_at, updated_at, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		im.a.TreeID, repo, call, collapse(title), author, publ, notes, xref, extraJSON(kept), im.now, im.now,
		nullID(im.a.UserID), nullID(im.a.UserID))
	if err != nil {
		return 0, err
	}
	im.report.Counts["sources"]++
	return res.LastInsertId()
}

// citation imports a SOUR line under a person, name, event or family.
func (im *importer) citation(ctx context.Context, ctxPath string, c *gedcom.Node, entityType string, entityID int64) error {
	return im.citationField(ctx, ctxPath, c, entityType, entityID, "")
}

// citationField is citation for a source of one field of the entity, such
// as the main name of a person (field "name").
func (im *importer) citationField(ctx context.Context, ctxPath string, c *gedcom.Node, entityType string, entityID int64, field string) error {
	sourceID, ok := im.sources[c.Pointer]
	var kept []keptNode
	if c.Pointer == "" {
		// GEDCOM 5.5 allowed sources written inline; each becomes a source.
		if strings.TrimSpace(c.Value) == "" {
			return nil
		}
		id, err := im.createSource(ctx, truncate(c.Value, 200), "", "", "", c.Value, sql.NullInt64{}, "", nil)
		if err != nil {
			return err
		}
		sourceID, ok = id, true
	}
	if !ok {
		im.broken(c, RefSource)
		return nil
	}
	page, text := "", ""
	var notes []string
	var quality sql.NullInt64
	for _, sub := range c.Children {
		switch sub.Tag {
		case "PAGE":
			page = sub.Value
			im.mapped(ctxPath + ".SOUR.PAGE")
		case "QUAY":
			if q, err := strconv.Atoi(strings.TrimSpace(sub.Value)); err == nil && q >= 0 && q <= 3 {
				quality = sql.NullInt64{Int64: int64(q), Valid: true}
				im.mapped(ctxPath + ".SOUR.QUAY")
			} else {
				im.keep(ctxPath+".SOUR", sub, &kept)
			}
		case "DATA":
			if t := sub.Text("TEXT"); t != "" {
				text = t
				im.mapped(ctxPath + ".SOUR.DATA.TEXT")
			}
			for _, d := range sub.Children {
				if d.Tag != "TEXT" {
					im.keep(ctxPath+".SOUR.DATA", d, &kept)
				}
			}
		case "TEXT":
			text = sub.Value
			im.mapped(ctxPath + ".SOUR.TEXT")
		case "NOTE", "SNOTE":
			notes = append(notes, im.noteText(sub))
			im.mapped(ctxPath + ".SOUR." + sub.Tag)
		default:
			im.other(ctxPath+".SOUR", sub, &kept)
		}
	}
	res, err := im.tx.ExecContext(ctx, `
		INSERT INTO citations (tree_id, source_id, page, quality, text, notes, extra_json, created_at, updated_at, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		im.a.TreeID, sourceID, page, quality, text, joinNotes(notes), extraJSON(kept), im.now, im.now,
		nullID(im.a.UserID), nullID(im.a.UserID))
	if err != nil {
		return err
	}
	cid, _ := res.LastInsertId()
	if _, err := im.tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO citation_links (citation_id, entity_type, entity_id, field) VALUES (?, ?, ?, ?)`, cid, entityType, entityID, field); err != nil {
		return err
	}
	im.report.Counts["citations"]++
	im.mapped(ctxPath + ".SOUR")
	return nil
}

// parsedName splits a GEDCOM name "Given /Surname/ Suffix" and applies the
// GIVN/SURN/… parts, which win over the slashes.
type parsedName struct {
	given, surname, prefix, suffix, nickname, nameType string
	status, statusReason                               string
	// path is the tag of the name in the report (INDI.NAME or
	// INDI._MARNM); its notes are reported by person(), which knows
	// whether the name is kept.
	path     string
	notes    []string
	noteTags []string
	sources  []*gedcom.Node
}

func (im *importer) name(n *gedcom.Node, kept *[]keptNode, ctx string, primary bool) parsedName {
	p := parsedName{path: ctx + ".NAME"}
	v := n.Value
	if i := strings.Index(v, "/"); i >= 0 {
		p.given = strings.TrimSpace(v[:i])
		rest := v[i+1:]
		if j := strings.Index(rest, "/"); j >= 0 {
			p.surname = strings.TrimSpace(rest[:j])
			p.suffix = strings.TrimSpace(rest[j+1:])
		} else {
			p.surname = strings.TrimSpace(rest)
		}
	} else {
		p.given = strings.TrimSpace(v)
	}
	spfx := ""
	for _, c := range n.Children {
		switch c.Tag {
		case "GIVN":
			p.given = c.Value
		case "SURN":
			p.surname = c.Value
		case "NPFX":
			p.prefix = c.Value
		case "NSFX":
			p.suffix = c.Value
		case "NICK":
			p.nickname = c.Value
		case "SPFX":
			spfx = c.Value
		case "TYPE":
			p.nameType = c.Value
			// 7.0 writes religious and other names as TYPE OTHER + PHRASE.
			if strings.EqualFold(strings.TrimSpace(c.Value), "OTHER") {
				if ph := strings.TrimSpace(c.Text("PHRASE")); ph != "" {
					p.nameType = ph
					im.mapped(ctx + ".NAME.TYPE.PHRASE")
				}
			}
		case "_STAT":
			st := strings.ToLower(strings.TrimSpace(c.Value))
			switch {
			case primary && st == StatusAccepted && len(c.Children) == 0:
				im.mapped(ctx + ".NAME._STAT") // nothing to keep
			case primary && validStatus(st) && st != StatusAccepted:
				// The main name lives on the person, which has no status.
				im.tag(ctx+".NAME._STAT", TagDropped, DropNameStatus)
			case !primary && p.status == "":
				if s, reason, ok := im.factStatus(ctx+".NAME", c); ok {
					p.status, p.statusReason = s, reason
					break
				}
				im.other(ctx+".NAME", c, kept)
			default:
				im.other(ctx+".NAME", c, kept)
			}
			continue
		case "NOTE", "SNOTE":
			p.notes, p.noteTags = append(p.notes, im.noteText(c)), append(p.noteTags, c.Tag)
			continue
		case "SOUR":
			p.sources = append(p.sources, c) // cited by person()
			continue
		default:
			im.other(ctx+".NAME", c, kept)
			continue
		}
		im.mapped(ctx + ".NAME." + c.Tag)
	}
	if spfx != "" && !strings.HasPrefix(p.surname, spfx) {
		p.surname = strings.TrimSpace(spfx + " " + p.surname)
	}
	p.given, p.surname = collapse(p.given), collapse(p.surname)
	return p
}

func nameType(gedcomType, tag string) string {
	if tag == "_MARNM" {
		return "married"
	}
	switch strings.ToLower(strings.TrimSpace(gedcomType)) {
	case "birth", "maiden":
		return "birth"
	case "married":
		return "married"
	case "aka", "alias", "also known as", "nickname":
		return "aka"
	case "immigrant":
		return "immigrant"
	case "religious":
		return "religious"
	default:
		return "other"
	}
}

func (im *importer) person(ctx context.Context, r *gedcom.Node) error {
	var kept []keptNode
	var notes []string
	var primary *parsedName
	var others []parsedName
	sex := "U"
	var events, citations, assos, famcs []*gedcom.Node

	for _, c := range r.Children {
		switch {
		case c.Tag == "NAME":
			p := im.name(c, &kept, "INDI", primary == nil)
			if primary == nil {
				primary = &p
			} else {
				p.nameType = nameType(p.nameType, c.Tag)
				others = append(others, p)
			}
			im.mapped("INDI.NAME")
		case c.Tag == "_MARNM":
			// Ancestry's married name: "1 _MARNM Schmidt" or "/Schmidt/".
			p := parsedName{surname: strings.Trim(strings.TrimSpace(c.Value), "/"), nameType: "married", path: "INDI._MARNM"}
			for _, sub := range c.Children {
				switch sub.Tag {
				case "SOUR":
					p.sources = append(p.sources, sub)
				case "NOTE", "SNOTE":
					p.notes, p.noteTags = append(p.notes, im.noteText(sub)), append(p.noteTags, sub.Tag)
				default:
					im.tag("INDI._MARNM."+sub.Tag, TagDropped, DropMarriedNameField)
				}
			}
			others = append(others, p)
			im.mapped("INDI._MARNM")
		case c.Tag == "SEX":
			switch v := strings.ToUpper(strings.TrimSpace(c.Value)); v {
			case "M", "F", "X", "U":
				sex = v
			}
			im.mapped("INDI.SEX")
		case personEventTags[c.Tag] || attributeTags[c.Tag]:
			events = append(events, c)
		case c.Tag == "NOTE" || c.Tag == "SNOTE":
			notes = append(notes, im.noteText(c))
			im.mapped("INDI." + c.Tag)
		case c.Tag == "SOUR":
			citations = append(citations, c)
		case c.Tag == "ASSO" && sharedEventRela.MatchString(c.Text("RELA")):
			assos = append(assos, c)
		case c.Tag == "FAMC":
			if _, ok := im.doc.RecordByXref(c.Pointer); !ok && c.Pointer != "VOID" {
				im.broken(c, RefFamily)
			}
			if c.Pointer != "" && c.Pointer != "VOID" {
				famcs = append(famcs, c)
			}
			im.mapped("INDI.FAMC")
		default:
			im.other("INDI", c, &kept)
		}
	}
	if primary == nil {
		primary = &parsedName{}
	}
	res, err := im.insertPerson.ExecContext(ctx, im.a.TreeID, primary.given, primary.surname, primary.prefix, primary.suffix,
		primary.nickname, sex, joinNotes(notes), r.Xref, extraJSON(kept), im.now, im.now, nullID(im.a.UserID), nullID(im.a.UserID),
		db.NamesKey(primary.surname), db.NamesKey(primary.given), joinNotes(primary.notes))
	if err != nil {
		return err
	}
	pid, _ := res.LastInsertId()
	im.persons[r.Xref] = pid
	for _, c := range famcs {
		im.famcLinks = append(im.famcLinks, famcLink{childID: pid, childXref: r.Xref, family: c.Pointer, pedi: c.Text("PEDI"), line: c.Line})
	}
	im.report.Counts["persons"]++
	im.mapped("INDI")

	allNames := []string{primary.given, primary.surname, primary.nickname}
	for _, tag := range primary.noteTags {
		im.mapped(primary.path + "." + tag)
	}
	for i, n := range others {
		if n.given == "" && n.surname == "" && n.nickname == "" {
			for range n.sources {
				im.tag(n.path+".SOUR", TagDropped, DropNameEmpty)
			}
			for _, tag := range n.noteTags {
				im.tag(n.path+"."+tag, TagDropped, DropNameEmpty)
			}
			continue
		}
		for _, tag := range n.noteTags {
			im.mapped(n.path + "." + tag)
		}
		res, err := im.insertName.ExecContext(ctx, pid, n.nameType, n.given, n.surname, n.prefix, n.suffix, n.nickname,
			defaultStatus(n.status), n.statusReason, i, joinNotes(n.notes))
		if err != nil {
			return err
		}
		nameID, _ := res.LastInsertId()
		for _, c := range n.sources {
			if err := im.citation(ctx, n.path, c, "name", nameID); err != nil {
				return err
			}
		}
		allNames = append(allNames, n.given, n.surname, n.nickname)
	}
	if _, err := im.insertFTS.ExecContext(ctx, pid, searchFold.Replace(strings.Join(allNames, " "))); err != nil {
		return err
	}
	for _, c := range primary.sources {
		if err := im.citationField(ctx, "INDI.NAME", c, "person", pid, "name"); err != nil {
			return err
		}
	}
	for _, c := range citations {
		if err := im.citation(ctx, "INDI", c, "person", pid); err != nil {
			return err
		}
	}
	eventIDs := map[string][]int64{}
	for i, e := range events {
		id, err := im.event(ctx, "INDI", e, sql.NullInt64{Int64: pid, Valid: true}, sql.NullInt64{}, i)
		if err != nil {
			return err
		}
		eventIDs[e.Tag] = append(eventIDs[e.Tag], id)
	}
	// GoTree writes shared-event participants in 5.5.1 as
	// "ASSO @I2@ / RELA godparent (BAPM)" on the principal, and those of a
	// family event as "RELA witness (MARR F3)" on a partner.
	keptAsso := false
	for _, asso := range assos {
		m := sharedEventRela.FindStringSubmatch(asso.Text("RELA"))
		role, custom, tag, famXref := m[1], m[2], m[3], m[4]
		ordinal := 1
		if m[5] != "" {
			ordinal, _ = strconv.Atoi(m[5])
		}
		if asso.Pointer == "" || asso.Pointer == "VOID" {
			im.keep("INDI", asso, &kept)
			keptAsso = true
			continue
		}
		var notes []string
		for _, c := range asso.Children {
			switch c.Tag {
			case "NOTE", "SNOTE":
				notes = append(notes, im.noteText(c))
				im.mapped("INDI.ASSO." + c.Tag)
			case "SOUR":
				im.tag("INDI.ASSO.SOUR", TagDropped, DropAssoSources)
			}
		}
		d := deferredParticipant{xref: asso.Pointer, role: role, custom: collapse(custom), notes: joinNotes(notes), line: asso.Line}
		if famXref != "" {
			d.famXref, d.tag, d.ordinal = famXref, tag, ordinal
		} else {
			ids := eventIDs[tag]
			if ordinal < 1 || ordinal > len(ids) {
				im.keep("INDI", asso, &kept)
				keptAsso = true
				continue
			}
			d.eventID = ids[ordinal-1]
		}
		im.deferred = append(im.deferred, d)
		im.mapped("INDI.ASSO")
	}
	if keptAsso {
		// The lines kept above came after the person was inserted.
		if _, err := im.tx.ExecContext(ctx, `UPDATE persons SET extra_json = ? WHERE id = ?`, extraJSON(kept), pid); err != nil {
			return err
		}
	}
	return nil
}

var sharedEventRela = regexp.MustCompile(`^(head|spouse|child|parent|sibling|relative|witness|godparent|informant|officiant|clergy|friend|neighbor|other)(?:: (.*?))? \(([A-Z_][A-Z0-9_]*)(?: ([A-Za-z][A-Za-z0-9_]*))?(?: ([0-9]+))?\)$`)

var assoRoles = map[string]string{
	"CHIL": "child", "CLERGY": "clergy", "FATH": "parent", "FRIEND": "friend", "GODP": "godparent", "HUSB": "spouse",
	"MOTH": "parent", "NGHBR": "neighbor", "OFFICIATOR": "officiant", "PARENT": "parent", "SPOU": "spouse",
	"WIFE": "spouse", "WITN": "witness", "OTHER": "other",
}

// assoRole reads the role of a 7.0 ASSO. 7.0 has no head, sibling, relative
// or informant, so GoTree writes them as ROLE OTHER with the role in a
// PHRASE, followed by ": custom" when the participant has a name of their
// own. A ROLE 7.0 knows keeps its PHRASE as the custom name. Without a ROLE
// (some 5.5.1 writers put the role in RELA) the RELA is used.
func (im *importer) assoRole(path string, a *gedcom.Node) (role, custom string) {
	rn := a.First("ROLE")
	if rn == nil {
		rela := collapse(a.Text("RELA"))
		if rela == "" {
			return "other", ""
		}
		im.mapped(path + ".ASSO.RELA")
		if r := strings.ToLower(rela); oneOf(r, participantRoles...) {
			return r, ""
		}
		return "other", rela
	}
	phrase := collapse(rn.Text("PHRASE"))
	if phrase != "" {
		im.mapped(path + ".ASSO.ROLE.PHRASE")
	}
	return rolePhrase(assoRoles[strings.ToUpper(strings.TrimSpace(rn.Value))], phrase)
}

// rolePhrase turns a 7.0 ROLE (already mapped to a GoTree role, "" when
// unknown) and its PHRASE into a role and a custom name.
func rolePhrase(role, phrase string) (string, string) {
	if role != "" && role != "other" {
		return role, phrase
	}
	if phrase == "" {
		return "other", ""
	}
	head, rest, _ := strings.Cut(phrase, ": ")
	if r := strings.ToLower(strings.TrimSpace(head)); oneOf(r, participantRoles...) {
		if rest = strings.TrimSpace(rest); rest != "" || !strings.Contains(phrase, ": ") {
			return r, rest
		}
	}
	return "other", phrase
}

func (im *importer) event(ctx context.Context, owner string, e *gedcom.Node, personID, familyID sql.NullInt64, order int) (int64, error) {
	path := owner + "." + e.Tag
	var kept []keptNode
	var notes []string
	var citations []*gedcom.Node
	typ := e.Tag
	if typ == "_MIL" {
		typ = "_MILT"
	}
	label, description, dateRaw := "", "", ""
	var placeID sql.NullInt64
	if attributeTags[e.Tag] || (e.Value != "" && e.Value != "Y") {
		description = e.Value
	}
	var assos []*gedcom.Node
	status, statusReason, statusSeen := StatusAccepted, "", false
	for _, c := range e.Children {
		switch c.Tag {
		case "_STAT":
			if !statusSeen {
				if st, reason, ok := im.factStatus(path, c); ok {
					status, statusReason, statusSeen = st, reason, true
					break // leaves the switch
				}
			}
			im.keep(path, c, &kept)
		case "DATE":
			var used bool
			dateRaw, used = eventDateText(c)
			im.mapped(path + ".DATE")
			for _, ph := range c.Children {
				if ph.Tag != "PHRASE" {
					continue
				}
				if used {
					im.mapped(path + ".DATE.PHRASE")
				} else {
					im.keep(path+".DATE", ph, &kept)
				}
			}
		case "PLAC":
			id, err := im.place(ctx, c, path, &kept)
			if err != nil {
				return 0, err
			}
			placeID = id
		case "TYPE":
			if typ == "EVEN" || typ == "FACT" {
				label = c.Value
			} else if description == "" {
				description = c.Value
			} else {
				im.keep(path, c, &kept)
				continue
			}
			im.mapped(path + ".TYPE")
		case "CAUS":
			if description == "" {
				description = c.Value
				im.mapped(path + ".CAUS")
			} else {
				im.keep(path, c, &kept)
			}
		case "NOTE", "SNOTE":
			notes = append(notes, im.noteText(c))
			im.mapped(path + "." + c.Tag)
		case "SOUR":
			citations = append(citations, c)
		case "ASSO":
			if c.Pointer == "" || c.Pointer == "VOID" {
				// Nobody to point at: keep the line instead of reporting a
				// broken reference.
				im.keep(path, c, &kept)
				break
			}
			assos = append(assos, c)
		default:
			im.other(path, c, &kept)
		}
	}
	if (typ == "EVEN" || typ == "FACT") && label == "" {
		label = description
		if label == "" {
			label = "Event"
		}
	}
	date, key, keyEnd, qual := parseDate(dateRaw)
	if dateRaw != "" && date == "" {
		im.report.InvalidDates++
		if len(im.report.InvalidDateExamples) < 10 {
			im.report.InvalidDateExamples = append(im.report.InvalidDateExamples, gedcom.Warning{
				Line: e.Line, Code: ReportInvalidDate, Params: map[string]any{"date": dateRaw}, Message: dateRaw,
			})
		}
	}
	res, err := im.insertEvent.ExecContext(ctx, im.a.TreeID, personID, familyID, typ, label, dateRaw, date, key, keyEnd, qual,
		placeID, description, joinNotes(notes), status, statusReason, order, extraJSON(kept), im.now, im.now, nullID(im.a.UserID), nullID(im.a.UserID))
	if err != nil {
		return 0, err
	}
	eid, _ := res.LastInsertId()
	im.eventNodes[e] = eid
	im.report.Counts["events"]++
	im.mapped(path)
	for _, c := range citations {
		if err := im.citation(ctx, path, c, "event", eid); err != nil {
			return 0, err
		}
	}
	for _, a := range assos {
		role, custom := im.assoRole(path, a)
		var notes []string
		for _, c := range a.Children {
			switch c.Tag {
			case "NOTE", "SNOTE":
				notes = append(notes, im.noteText(c))
				im.mapped(path + ".ASSO." + c.Tag)
			case "SOUR":
				im.tag(path+".ASSO.SOUR", TagDropped, DropAssoSources)
			}
		}
		im.deferred = append(im.deferred, deferredParticipant{eventID: eid, xref: a.Pointer, role: role, custom: custom, notes: joinNotes(notes), line: a.Line})
		im.mapped(path + ".ASSO")
	}
	return eid, nil
}

// place turns "Leipzig, Sachsen, Deutschland" into the place hierarchy,
// reusing places already created, and returns the most specific one.
func (im *importer) place(ctx context.Context, n *gedcom.Node, path string, kept *[]keptNode) (sql.NullInt64, error) {
	var parts []string
	for _, p := range strings.Split(n.Value, ",") {
		if p = collapse(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return sql.NullInt64{}, nil
	}
	var parent sql.NullInt64
	for i := len(parts) - 1; i >= 0; i-- {
		key := fmt.Sprintf("%d|%s", parent.Int64, strings.ToLower(parts[i]))
		if id, ok := im.places[key]; ok {
			parent = sql.NullInt64{Int64: id, Valid: true}
			continue
		}
		res, err := im.tx.ExecContext(ctx, `
			INSERT INTO places (tree_id, parent_id, name, created_at, updated_at, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			im.a.TreeID, parent, parts[i], im.now, im.now, nullID(im.a.UserID), nullID(im.a.UserID))
		if err != nil {
			return sql.NullInt64{}, err
		}
		id, _ := res.LastInsertId()
		im.places[key] = id
		im.report.Counts["places"]++
		parent = sql.NullInt64{Int64: id, Valid: true}
	}
	im.mapped(path + ".PLAC")

	for _, c := range n.Children {
		switch c.Tag {
		case "MAP":
			lat, ok1 := coordinate(c.Text("LATI"), 'N', 'S')
			lng, ok2 := coordinate(c.Text("LONG"), 'E', 'W')
			if ok1 && ok2 && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 {
				if _, err := im.tx.ExecContext(ctx, `UPDATE places SET lat = ?, lng = ? WHERE id = ? AND lat IS NULL`, lat, lng, parent.Int64); err != nil {
					return sql.NullInt64{}, err
				}
				im.mapped(path + ".PLAC.MAP")
			} else {
				im.keep(path+".PLAC", c, kept)
			}
		case "FORM":
			im.tag(path+".PLAC.FORM", TagDropped, DropPlaceForm)
		default:
			im.keep(path+".PLAC", c, kept)
		}
	}
	return parent, nil
}

// coordinate reads "N50.123" / "S12.5" style GEDCOM coordinates.
func coordinate(v string, pos, neg byte) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	sign := 1.0
	switch v[0] {
	case pos:
		v = v[1:]
	case neg:
		sign, v = -1, v[1:]
	}
	f, err := strconv.ParseFloat(v, 64)
	return sign * f, err == nil
}

var pediRelations = map[string]string{
	"BIRTH": "birth", "ADOPTED": "adopted", "FOSTER": "foster", "SEALING": "sealing", "OTHER": "unknown",
	"NATURAL": "birth", "STEP": "step", "UNKNOWN": "unknown", "GUARDIAN": "foster", "PRIVATE": "unknown",
	"SURROGATE": "surrogate",
}

func relation(v string) string {
	if r, ok := pediRelations[strings.ToUpper(strings.TrimSpace(v))]; ok {
		return r
	}
	return "birth"
}

func (im *importer) family(ctx context.Context, r *gedcom.Node) error {
	var kept []keptNode
	var notes []string
	var p1, p2 sql.NullInt64
	var children, events, citations []*gedcom.Node
	married := false
	for _, c := range r.Children {
		switch {
		case c.Tag == "HUSB" || c.Tag == "WIFE":
			id, ok := im.persons[c.Pointer]
			if !ok {
				if c.Pointer != "VOID" {
					im.broken(c, RefPartner)
				}
				continue
			}
			target := &p1
			if c.Tag == "WIFE" {
				target = &p2
			}
			if target.Valid {
				im.keep("FAM", c, &kept)
				continue
			}
			*target = sql.NullInt64{Int64: id, Valid: true}
			im.mapped("FAM." + c.Tag)
		case c.Tag == "CHIL":
			im.chilSeen[r.Xref+"\x00"+c.Pointer] = true
			children = append(children, c)
		case familyEventTagsIn[c.Tag]:
			if c.Tag == "MARR" {
				married = true
			}
			events = append(events, c)
		case c.Tag == "NOTE" || c.Tag == "SNOTE":
			notes = append(notes, im.noteText(c))
			im.mapped("FAM." + c.Tag)
		case c.Tag == "SOUR":
			citations = append(citations, c)
		default:
			im.other("FAM", c, &kept)
		}
	}
	// Someone recorded twice in one family (HUSB and WIFE) would violate
	// the schema; the second mention is kept verbatim instead.
	if p1.Valid && p2.Valid && p1.Int64 == p2.Int64 {
		p2 = sql.NullInt64{}
	}
	union := "unknown"
	if married {
		union = "married"
	}
	res, err := im.tx.ExecContext(ctx, `
		INSERT INTO families (tree_id, partner1_id, partner2_id, union_type, notes, gedcom_xref, extra_json, created_at, updated_at, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		im.a.TreeID, p1, p2, union, joinNotes(notes), r.Xref, extraJSON(kept), im.now, im.now, nullID(im.a.UserID), nullID(im.a.UserID))
	if err != nil {
		return err
	}
	fid, _ := res.LastInsertId()
	im.families[r.Xref] = fid
	im.familyPartners[r.Xref] = [2]sql.NullInt64{p1, p2}
	im.report.Counts["families"]++
	im.mapped("FAM")

	for i, c := range children {
		cid, ok := im.persons[c.Pointer]
		if !ok {
			im.broken(c, RefChild)
			continue
		}
		if (p1.Valid && cid == p1.Int64) || (p2.Valid && cid == p2.Int64) {
			im.report.BrokenReferences = append(im.report.BrokenReferences, partnerAndChild(c.Line, c.Pointer, r.Xref))
			continue
		}
		rel := "birth"
		if pedi, ok := im.pedigree[c.Pointer][r.Xref]; ok {
			rel = relation(pedi)
		}
		rel1, rel2 := rel, rel
		if v := c.Text("_FREL"); v != "" {
			rel1 = relation(v)
			im.mapped("FAM.CHIL._FREL")
		}
		if v := c.Text("_MREL"); v != "" {
			rel2 = relation(v)
			im.mapped("FAM.CHIL._MREL")
		}
		if _, err := im.tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO family_children (family_id, child_id, relation_partner1, relation_partner2, sort_order)
			VALUES (?, ?, ?, ?, ?)`, fid, cid, rel1, rel2, i); err != nil {
			return err
		}
		im.mapped("FAM.CHIL")
	}
	for _, c := range citations {
		if err := im.citation(ctx, "FAM", c, "family", fid); err != nil {
			return err
		}
	}
	for i, e := range events {
		id, err := im.event(ctx, "FAM", e, sql.NullInt64{}, sql.NullInt64{Int64: fid, Valid: true}, i)
		if err != nil {
			return err
		}
		if im.familyEvents[r.Xref] == nil {
			im.familyEvents[r.Xref] = map[string][]int64{}
		}
		im.familyEvents[r.Xref][e.Tag] = append(im.familyEvents[r.Xref][e.Tag], id)
	}
	return nil
}

// linkFAMCChildren adds the children that name a family in their own FAMC
// line but are not listed under the family's CHIL.
func (im *importer) linkFAMCChildren(ctx context.Context) error {
	for _, l := range im.famcLinks {
		fid, ok := im.families[l.family]
		if !ok {
			continue
		}
		if l.childXref != "" && im.chilSeen[l.family+"\x00"+l.childXref] {
			continue
		}
		partners := im.familyPartners[l.family]
		if (partners[0].Valid && partners[0].Int64 == l.childID) || (partners[1].Valid && partners[1].Int64 == l.childID) {
			if len(im.report.BrokenReferences) < 50 {
				im.report.BrokenReferences = append(im.report.BrokenReferences, partnerAndChild(l.line, l.childXref, l.family))
			}
			continue
		}
		pedi := l.pedi
		if p := im.pedigree[l.childXref][l.family]; p != "" {
			pedi = p
		}
		rel := relation(pedi)
		var order int
		if err := im.tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), -1) + 1 FROM family_children WHERE family_id = ?`, fid).Scan(&order); err != nil {
			return err
		}
		res, err := im.tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO family_children (family_id, child_id, relation_partner1, relation_partner2, sort_order)
			VALUES (?, ?, ?, ?, ?)`, fid, l.childID, rel, rel, order)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		im.report.ChildrenFromFAMC++
		if len(im.report.ChildrenFromFAMCExamples) < 20 {
			w := gedcom.Warning{
				Line: l.line, Code: ReportFamcChildUnnamed, Params: map[string]any{"family": l.family},
				Message: fmt.Sprintf("a person added as a child of family @%s@", l.family),
			}
			if l.childXref != "" {
				w.Code = ReportFamcChild
				w.Params["xref"] = l.childXref
				w.Message = fmt.Sprintf("@%s@ added as a child of family @%s@", l.childXref, l.family)
			}
			im.report.ChildrenFromFAMCExamples = append(im.report.ChildrenFromFAMCExamples, w)
		}
	}
	return nil
}

// eventDateText reads a DATE line together with the DATE.PHRASE that
// GoTree's 7.0 export writes beside it: the text of a date that is not a
// date, the explanation of an INT date, or a dual date, whose value carries
// the later year (older GoTree files: the earlier one).
// usedPhrase says whether the phrase went into the text; when it did not,
// the caller has to keep it elsewhere.
func eventDateText(n *gedcom.Node) (text string, usedPhrase bool) {
	phrase := strings.TrimSpace(n.Text("PHRASE"))
	value := strings.TrimSpace(n.Value)
	if value == "" {
		if phrase == "" {
			return "", false
		}
		return "(" + phrase + ")", true
	}
	if phrase == "" {
		return value, false
	}
	d, err := gendate.Parse(value)
	if err != nil || d.Qualifier == gendate.QualifierPhrase || d.Qualifier == gendate.QualifierInterpreted {
		return value, false
	}
	// The phrase of a dual date is the date as 5.5.1 writes it ("12 FEB 1750/51").
	// The value carries the later year ("12 FEB 1751"); files written by older
	// GoTree versions carry the earlier one ("12 FEB 1750"), so both are read.
	if p, err := gendate.Parse(phrase); err == nil && p.Qualifier == gendate.QualifierNone && p.Start.DualYear != 0 {
		for _, sd := range []*gendate.SimpleDate{d.End, d.Start} {
			if sd != nil && sd.DualYear == 0 && sd.Calendar == p.Start.Calendar &&
				(sd.Year == p.Start.DualYear || sd.Year == p.Start.Year) &&
				sd.Month == p.Start.Month && sd.Day == p.Start.Day && sd.BC == p.Start.BC {
				sd.Year, sd.DualYear = p.Start.Year, p.Start.DualYear
				return d.String(), true
			}
		}
	}
	if d.Qualifier != gendate.QualifierNone {
		return value, false
	}
	d.Qualifier, d.Phrase = gendate.QualifierInterpreted, phrase
	return d.String(), true
}

// heirloom reads a GoTree _HEIRLOOM record (docs/gedcom-extensions.md).
func (im *importer) heirloom(ctx context.Context, r *gedcom.Node) error {
	const path = "_HEIRLOOM"
	in := HeirloomInput{Kind: "other"}
	var place sql.NullInt64
	var notes []string
	var citations []*gedcom.Node
	for _, c := range r.Children {
		switch c.Tag {
		case "NAME":
			in.Name = c.Value
		case "TYPE":
			if oneOf(c.Value, heirloomKinds...) {
				in.Kind = c.Value
			}
		case "_DESC":
			in.Description = c.Value
		case "DATE":
			text, used := eventDateText(c)
			in.MadeDate = canonicalDate(text)
			if c.First("PHRASE") != nil && !used {
				im.tag(path+".DATE.PHRASE", TagDropped, DropHeirloomDatePhrase)
			}
		case "PLAC":
			// Heirlooms have nowhere to keep unknown lines; place
			// substructures GoTree does not read are lost here.
			var ignored []keptNode
			p, err := im.place(ctx, c, path, &ignored)
			if err != nil {
				return err
			}
			place = p
			continue
		case "_LOC":
			in.CurrentLocation = c.Value
		case "NOTE", "SNOTE":
			notes = append(notes, im.noteText(c))
		case "_CUST":
			cust := CustodyInput{How: "other"}
			if c.Pointer != "" {
				if id, ok := im.persons[c.Pointer]; ok {
					cust.PersonID = &id
				} else {
					im.broken(c, RefHolder)
				}
			}
			for _, sub := range c.Children {
				switch sub.Tag {
				case "_FROM":
					cust.FromDate = sub.Value
				case "_TO":
					cust.ToDate = sub.Value
				case "TYPE":
					if oneOf(sub.Value, custodyWays...) {
						cust.How = sub.Value
					}
				case "NOTE":
					cust.Notes = sub.Value
				default:
					im.tag(path+"._CUST."+sub.Tag, TagDropped, DropHeirloomCustody)
					continue
				}
				im.mapped(path + "._CUST." + sub.Tag)
			}
			if cust.PersonID == nil && cust.Notes == "" {
				cust.Notes = "?"
			}
			in.Custody = append(in.Custody, cust)
		case "SOUR":
			citations = append(citations, c)
			continue
		case "OBJE":
			if im.objeFate(c) != objeImported {
				im.tag(path+".OBJE", TagDropped, DropHeirloomMedia)
			}
			continue // otherwise linked by linkMedia
		default:
			im.tag(path+"."+c.Tag, TagDropped, DropHeirloomField)
			continue
		}
		im.mapped(path + "." + c.Tag)
	}
	in.Notes = joinNotes(notes)
	if place.Valid {
		in.OriginPlaceID = &place.Int64
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = "?"
	}
	in.normalizeWith(canonicalDate)
	now := im.s.now()
	res, err := im.tx.ExecContext(ctx, `
		INSERT INTO heirlooms (tree_id, name, kind, description, made_date_raw, made_date_sort, origin_place_id, current_location, notes,
			gedcom_xref, created_at, updated_at, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		im.a.TreeID, truncate(in.Name, 200), in.Kind, in.Description, in.MadeDate, madeSort(in.MadeDate), nullIDPtr(in.OriginPlaceID),
		in.CurrentLocation, in.Notes, r.Xref, now, now, nullID(im.a.UserID), nullID(im.a.UserID))
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	im.heirlooms[r.Xref] = id
	if err := writeCustody(ctx, im.tx, id, in.Custody); err != nil {
		return err
	}
	for _, c := range citations {
		if err := im.citation(ctx, path, c, "heirloom", id); err != nil {
			return err
		}
	}
	im.mapped(path)
	im.report.Counts["heirlooms"]++
	return nil
}
