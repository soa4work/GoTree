package api

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/praetorianer777/gotree/internal/gedcom"
	"github.com/praetorianer777/gotree/internal/store"
)

// importGEDCOM reads an uploaded .ged file, or the .ged inside a zip
// (GEDZIP or a zipped export), and imports it.
func (s *Server) importGEDCOM(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = store.ImportIntoEmpty
	}
	// Replace wipes the tree, so it is the owner's call, as with share links.
	// Checked before the upload is read.
	if mode == store.ImportReplace && actorFrom(r).Role != store.RoleOwner {
		writeError(w, http.StatusForbidden, "only the owner of the tree can replace it; editors can import into an empty tree")
		return
	}
	s.limitUpload(w, r)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected a multipart upload")
		return
	}
	var data []byte
	var name string
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if !writeSlowUpload(w, err) {
				writeError(w, http.StatusBadRequest, "invalid upload")
			}
			return
		}
		if part.FormName() == "file" {
			name = part.FileName()
			data, err = io.ReadAll(part)
			part.Close()
			if err != nil {
				if writeSlowUpload(w, err) {
					return
				}
				writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("the file is larger than %d MB", s.MaxUpload>>20))
				return
			}
			break
		}
		part.Close()
	}
	if data == nil {
		writeError(w, http.StatusBadRequest, "no file in the upload")
		return
	}

	limit := s.maxGedcom()
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		inner, err := gedcomFromZip(data, limit)
		if errors.Is(err, errGedcomTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		data = inner
	} else if int64(len(data)) > limit {
		// Plain uploads are already capped by limitUpload, so this is a
		// backstop; the limit mostly applies to zips.
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("%v (over %d MB)", errGedcomTooLarge, limit>>20))
		return
	}
	doc, err := gedcom.Parse(data)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	report, err := s.Store.ImportGEDCOM(r.Context(), actorFrom(r), doc, mode)
	if errors.Is(err, store.ErrTreeNotEmpty) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	s.Log.Info("gedcom imported", "file", name, "persons", report.Counts["persons"], "ms", report.DurationMS)
	writeJSON(w, http.StatusOK, report)
}

// errGedcomTooLarge is returned when a GEDCOM file exceeds the size limit.
var errGedcomTooLarge = errors.New("the GEDCOM file is too large")

// maxGedcom is the largest GEDCOM file, unzipped, that an import accepts:
// four times the upload limit. Text GEDCOM zips well (typically 5-10x), so a
// legitimate zip within the upload limit fits, while a zip bomb is stopped
// long before it can exhaust memory. The limit follows GOTREE_MAX_UPLOAD_MB
// instead of being a fixed cap.
func (s *Server) maxGedcom() int64 {
	return 4 * s.MaxUpload
}

// gedcomFromZip returns the GEDCOM file inside a zip: gedcom.ged as
// GEDZIP names it, else the only .ged file. A file larger than limit bytes
// unzipped is refused with errGedcomTooLarge.
func gedcomFromZip(data []byte, limit int64) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("the zip file cannot be read: %w", err)
	}
	var found []*zip.File
	for _, f := range zr.File {
		if strings.EqualFold(path.Ext(f.Name), ".ged") {
			if strings.EqualFold(f.Name, "gedcom.ged") {
				found = []*zip.File{f}
				break
			}
			found = append(found, f)
		}
	}
	switch len(found) {
	case 0:
		return nil, errors.New("the zip file contains no .ged file")
	case 1:
	default:
		return nil, errors.New("the zip file contains several .ged files; upload the one you want")
	}
	// The declared size can lie, so the bounded read below is the real check;
	// this refuses an honest oversized entry without reading it.
	if found[0].UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("%w (over %d MB unzipped)", errGedcomTooLarge, limit>>20)
	}
	rc, err := found[0].Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// Guard against zip bombs. Read one byte past the limit so an oversized
	// file is refused instead of silently truncated.
	out, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > limit {
		return nil, fmt.Errorf("%w (over %d MB unzipped)", errGedcomTooLarge, limit>>20)
	}
	return out, nil
}
