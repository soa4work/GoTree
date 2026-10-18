package api

import (
	"archive/zip"
	"errors"
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
	// Exports without living people are meant for sharing, so they never carry
	// photo locations; a full export does unless the instance keeps them.
	strip := !s.KeepPhotoLocation || (q.Get("privacy") != "" && q.Get("privacy") != store.PrivacyAll)
	for _, m := range ex.Media {
		if err != nil {
			break
		}
		err = s.addToZip(zw, m, strip)
	}
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		// Headers are sent; the client gets a broken download, the log the reason.
		s.Log.Error("write gedzip", "err", err)
	}
}

func (s *Server) addToZip(zw *zip.Writer, m store.MediaFile, strip bool) error {
	var path string
	var err error
	if strip {
		path, err = s.Files.WithoutLocation(m.SHA256, m.Mime)
	} else {
		path, err = s.Files.Path(m.SHA256)
	}
	if errors.Is(err, os.ErrNotExist) {
		s.Log.Warn("media file missing from export", "sha", m.SHA256)
		return nil
	}
	if err != nil {
		if !strip {
			return err
		}
		// Fail closed: leave the file out rather than ship the original.
		s.Log.Error("media file left out of export", "sha", m.SHA256, "err", err)
		return nil
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

func (s *Server) verifyExport(w http.ResponseWriter, r *http.Request) {
	report, err := s.Store.VerifyExport(r.Context(), actorFrom(r), r.URL.Query().Get("version") == "7.0", s.Version)
	s.respond(w, http.StatusOK, report, err)
}
