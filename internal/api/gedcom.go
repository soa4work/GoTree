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

	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		inner, err := gedcomFromZip(data)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		data = inner
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

// gedcomFromZip returns the GEDCOM file inside a zip: gedcom.ged as
// GEDZIP names it, else the only .ged file.
func gedcomFromZip(data []byte) ([]byte, error) {
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
	rc, err := found[0].Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// Guard against zip bombs: the GEDCOM may not be larger than 1 GB.
	out, err := io.ReadAll(io.LimitReader(rc, 1<<30))
	return out, err
}
