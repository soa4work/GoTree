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
	tmpDir := filepath.Join(f.Root, "tmp")
	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		return Stored{}, err
	}
	tmp, err := os.CreateTemp(tmpDir, "upload-*")
	if err != nil {
		return Stored{}, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, limit+1))
	if err != nil {
		return Stored{}, err
	}
	if n > limit {
		return Stored{}, ErrTooLarge
	}
	if err := tmp.Sync(); err != nil {
		return Stored{}, err
	}
	if err := tmp.Close(); err != nil {
		return Stored{}, err
	}

	if check != nil {
		if err := check(tmp.Name()); err != nil {
			return Stored{}, err
		}
	}

	sha := hex.EncodeToString(h.Sum(nil))
	dst, _ := f.Path(sha)
	if _, err := os.Stat(dst); err == nil {
		return Stored{SHA256: sha, Size: n}, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return Stored{}, err
	}
	// Rename is atomic, so a concurrent reader never sees a partial file.
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return Stored{}, err
	}
	return Stored{SHA256: sha, Size: n}, nil
}

// Remove deletes a file and its thumbnails.
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
	return nil
}
