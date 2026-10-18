// Package media stores uploaded files by content hash, detects their type,
// reads photo metadata and renders thumbnails.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// Files is the on-disk store below Root:
//
//	Root/ab/cd/abcd…  the original, named by its SHA-256
//	Root/thumbs/…     generated thumbnails
//	Root/clean/…      copies of originals without their GPS position
//	Root/tmp/…        uploads in progress
type Files struct {
	Root string
}

// Stored describes a saved file.
type Stored struct {
	SHA256 string
	Size   int64
}

// ErrTooLarge is returned by Save when the upload exceeds the limit.
var ErrTooLarge = errors.New("file too large")

var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Path is where the file with the given hash lives.
func (f Files) Path(sha string) (string, error) {
	if !shaPattern.MatchString(sha) {
		return "", fmt.Errorf("invalid hash %q", sha)
	}
	return filepath.Join(f.Root, sha[:2], sha[2:4], sha), nil
}

// Save streams r to disk, at most limit bytes. A file that is already
// stored is kept as is.
func (f Files) Save(r io.Reader, limit int64) (Stored, error) {
	return f.SaveChecked(r, limit, nil)
}

// SaveChecked is Save with a check that runs on the finished temporary file
// before it reaches the store. When check returns an error, nothing is
// stored, not even if an identical upload races with this one.
func (f Files) SaveChecked(r io.Reader, limit int64, check func(path string) error) (Stored, error) {
	staged, err := f.Stage(r, limit, check)
	if err != nil {
		return Stored{}, err
	}
	defer staged.Discard()
	if err := staged.Commit(); err != nil {
		return Stored{}, err
	}
	return Stored{SHA256: staged.SHA256, Size: staged.Size}, nil
}

// Staged is an upload that has been written, hashed and checked but is not
// in the store yet. Splitting the two steps lets the caller take a lock on
// the hash for the short commit instead of for the whole transfer.
type Staged struct {
	SHA256 string
	Size   int64

	files Files
	tmp   string
}

// Stage streams r into a temporary file, at most limit bytes, hashes it and
// runs check on it. The caller must Commit or Discard the result.
func (f Files) Stage(r io.Reader, limit int64, check func(path string) error) (*Staged, error) {
	tmpDir := filepath.Join(f.Root, "tmp")
	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(tmpDir, "upload-*")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, ErrTooLarge
	}
	if err := tmp.Sync(); err != nil {
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if check != nil {
		if err := check(tmp.Name()); err != nil {
			return nil, err
		}
	}
	ok = true
	return &Staged{SHA256: hex.EncodeToString(h.Sum(nil)), Size: n, files: f, tmp: tmp.Name()}, nil
}

// Commit moves the file into the store; a file that is already stored is
// kept as is. Calling it again does nothing.
func (s *Staged) Commit() error {
	if s.tmp == "" {
		return nil
	}
	dst, err := s.files.Path(s.SHA256)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		s.Discard()
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	// Rename is atomic, so a concurrent reader never sees a partial file.
	if err := os.Rename(s.tmp, dst); err != nil {
		return err
	}
	s.tmp = ""
	return nil
}

// Discard removes the temporary file unless Commit moved it. It is safe to
// defer next to Stage.
func (s *Staged) Discard() {
	if s.tmp != "" {
		os.Remove(s.tmp)
		s.tmp = ""
	}
}

// Remove deletes a file, its thumbnails and its location-free copy.
func (f Files) Remove(sha string) error {
	p, err := f.Path(sha)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	thumbs, _ := filepath.Glob(filepath.Join(f.Root, "thumbs", sha[:2], sha+"-*"))
	for _, t := range thumbs {
		_ = os.Remove(t)
	}
	clean, _ := filepath.Glob(filepath.Join(f.Root, "clean", sha[:2], sha+"-*"))
	for _, c := range clean {
		_ = os.Remove(c)
	}
	return nil
}
