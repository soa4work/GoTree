package api

import (
	"archive/zip"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/praetorianer777/gotree/internal/store"
)

const restoreNote = `GoTree backup
=============

gotree.db  the database (all trees, users and settings)
media/     the uploaded photos, documents and recordings

To restore, stop GoTree, put gotree.db and the media folder into an empty
data directory (GOTREE_DATA_DIR, /data in Docker), and start GoTree again.
Thumbnails are not included; they are made again when first shown.
`

// backup streams a zip of a database snapshot and the media files. It
// covers the whole instance, so only administrators may download it.
func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	if sessionFrom(r.Context()).User.Role != store.UserRoleAdmin {
		writeError(w, http.StatusForbidden, CodeAdminOnlyBackup, "only administrators can download a backup")
		return
	}
	dir, err := os.MkdirTemp("", "gotree-backup-")
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	defer os.RemoveAll(dir)
	snapshot := filepath.Join(dir, "gotree.db")
	if err := s.Store.Snapshot(r.Context(), snapshot); err != nil {
		writeStoreError(w, s.Log, err)
		return
	}

	name := "gotree-backup-" + s.Store.Now().UTC().Format("20060102-1504") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	zw := zip.NewWriter(w)
	// The status is sent with the first byte, so a failure from here on can
	// only cut the download short; the zip is then incomplete and will not
	// open, which is the signal the user gets.
	if err := s.writeBackup(zw, snapshot); err != nil {
		s.Log.Error("backup failed", "err", err)
		return
	}
	if err := zw.Close(); err != nil {
		s.Log.Error("backup failed", "err", err)
	}
}

func (s *Server) writeBackup(zw *zip.Writer, snapshot string) error {
	note, err := zw.Create("RESTORE.txt")
	if err != nil {
		return err
	}
	if _, err := io.WriteString(note, restoreNote); err != nil {
		return err
	}
	if err := addFile(zw, snapshot, "gotree.db", zip.Deflate); err != nil {
		return err
	}
	root := s.Files.Root
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "tmp" || rel == "thumbs" {
				return filepath.SkipDir
			}
			return nil
		}
		// Photos and videos are compressed already; storing them is as
		// small and much faster.
		return addFile(zw, path, "media/"+rel, zip.Store)
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func addFile(zw *zip.Writer, path, name string, method uint16) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name, hdr.Method = strings.TrimPrefix(name, "/"), method
	dst, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, f)
	return err
}
