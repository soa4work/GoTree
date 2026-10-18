package api

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/praetorianer777/gotree/internal/gedcom"
	"github.com/praetorianer777/gotree/internal/store"
)

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// exportGEDCOM sends the tree as a .ged file, or with format=gedzip as a
// zip of the GEDCOM file and the media it refers to.
func (s *Server) exportGEDCOM(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v7 := q.Get("version") == "7.0"
	zipped := q.Get("format") == "gedzip"
	a := actorFrom(r)
	ex, err := s.Store.ExportGEDCOM(r.Context(), a, store.ExportOptions{
		Version7: v7, Privacy: q.Get("privacy"), WithMedia: zipped, AppVersion: s.Version,
	})
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}

	name, _ := s.Store.TreeName(r.Context(), a.TreeID)
	base := strings.Trim(strings.ToLower(unsafeName.ReplaceAllString(name, "-")), "-")
	if base == "" {
		base = "gotree"
	}
	base += "-" + time.Now().Format("2006-01-02")
	opt := gedcom.WriteOptions{Version7: v7}

	if !zipped {
		w.Header().Set("Content-Type", "text/vnd.familysearch.gedcom; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", base+".ged"))
		if err := gedcom.Write(w, ex.Records, opt); err != nil {
			s.Log.Error("write gedcom", "err", err)
		}
		return
	}

	// GEDZIP is defined for GEDCOM 7; a 5.5.1 archive is an ordinary zip.
	ext := ".zip"
	if v7 {
		ext = ".gdz"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", base+ext))
	zw := zip.NewWriter(w)
	gw, err := zw.Create("gedcom.ged")
	if err == nil {
		err = gedcom.Write(gw, ex.Records, opt)
	}
	for _, m := range ex.Media {
		if err != nil {
			break
		}
		err = s.addToZip(zw, m)
	}
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		// Headers are sent; the client gets a broken download, the log the reason.
		s.Log.Error("write gedzip", "err", err)
	}
}

func (s *Server) addToZip(zw *zip.Writer, m store.MediaFile) error {
	path, err := s.Files.Path(m.SHA256)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		s.Log.Warn("media file missing from export", "sha", m.SHA256)
		return nil
	}
	defer f.Close()
	// Photos and recordings are compressed already.
	zf, err := zw.CreateHeader(&zip.FileHeader{Name: m.Path, Method: zip.Store, Modified: time.Now()})
	if err != nil {
		return err
	}
	_, err = io.Copy(zf, f)
	return err
}

// verifyExport runs the export check. It exports, imports into a scratch
// database and compares, which is costly, so only editors may start it and
// only one check runs at a time on the whole instance.
func (s *Server) verifyExport(w http.ResponseWriter, r *http.Request) {
	if !actorFrom(r).CanEdit() {
		writeError(w, http.StatusForbidden, CodeViewOnly, "you can only view this tree")
		return
	}
	select {
	case s.verifySlot <- struct{}{}:
		defer func() { <-s.verifySlot }()
	default:
		w.Header().Set("Retry-After", "10")
		writeError(w, http.StatusTooManyRequests, CodeExportCheckBusy, "an export check is already running; try again in a moment")
		return
	}
	report, err := s.Store.VerifyExport(r.Context(), actorFrom(r), r.URL.Query().Get("version") == "7.0", s.Version)
	s.respond(w, http.StatusOK, report, err)
}
