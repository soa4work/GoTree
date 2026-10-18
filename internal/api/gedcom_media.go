package api

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/praetorianer777/gotree/internal/media"
	"github.com/praetorianer777/gotree/internal/store"
)

// Why a file of a GEDZIP was not imported; they end up in the import report.
const (
	skipWeb       = "links to files on the web are not imported"
	skipNotInZip  = "the file is not in the zip"
	skipTooLarge  = "the file is larger than the upload limit"
	skipBomb      = "the zip unpacks to more than the import limit"
	skipType      = "this kind of file is not supported"
	skipPixels    = "the image is larger than the pixel limit"
	skipUnreadble = "the file in the zip cannot be read"
)

// zipIndex finds the files of a zip by the paths a GEDCOM file writes.
type zipIndex struct {
	byName map[string]*zip.File   // lower-case path
	byBase map[string][]*zip.File // lower-case base name
	gedDir string
}

func newZipIndex(zr *zip.Reader, gedName string) *zipIndex {
	ix := &zipIndex{byName: map[string]*zip.File{}, byBase: map[string][]*zip.File{}, gedDir: path.Dir(gedName)}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") || f.Name == gedName || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		key := strings.ToLower(path.Clean(name))
		if _, dup := ix.byName[key]; !dup {
			ix.byName[key] = f
		}
		base := strings.ToLower(path.Base(key))
		ix.byBase[base] = append(ix.byBase[base], f)
	}
	return ix
}

// resolve finds the entry for a FILE value: relative to the GEDCOM file,
// relative to the root of the zip, or, for an absolute path from the
// computer that wrote the file, by its name when that is unambiguous.
func (ix *zipIndex) resolve(value string) (*zip.File, string) {
	lower := strings.ToLower(value)
	for _, scheme := range []string{"http://", "https://", "ftp://"} {
		if strings.HasPrefix(lower, scheme) {
			return nil, skipWeb
		}
	}
	v := strings.TrimPrefix(value, "file://")
	if len(v) != len(value) && strings.HasPrefix(v, "localhost/") {
		v = strings.TrimPrefix(v, "localhost")
	}
	v = strings.ReplaceAll(v, `\`, "/")
	if strings.Contains(v, "%") {
		if u, err := url.PathUnescape(v); err == nil {
			v = u
		}
	}
	for _, cand := range []string{
		path.Join(ix.gedDir, v),
		strings.TrimLeft(strings.TrimPrefix(v, "./"), "/"),
	} {
		if f, ok := ix.byName[strings.ToLower(path.Clean(cand))]; ok {
			return f, ""
		}
	}
	if list := ix.byBase[strings.ToLower(path.Base(v))]; len(list) == 1 && path.Base(v) != "." && path.Base(v) != "/" {
		return list[0], ""
	}
	return nil, skipNotInZip
}

// stagedFile is a zip entry that has been written to a temporary file.
type stagedFile struct {
	staged *media.Staged
	mime   string
	kind   media.Kind
	name   string
}

// stageGedzipMedia takes the files that the document names out of the zip
// with the checks of an upload: the same size limit, the same pixel limit
// and the same list of types. A file that fails a check is left out and
// reported with its reason; the import goes on without it. Everything the
// zip unpacks to is capped at the import limit, which stops a zip bomb.
//
// The files are committed to the store under the hash locks of an upload and
// stay locked until the returned finish is called. finish removes the files
// that no medium uses (all of them after a failed import), which is safe
// because the locks are still held. It must be called exactly once.
func (s *Server) stageGedzipMedia(ctx context.Context, zr *zip.Reader, gedName string, gedSize int, values []string) (store.ImportFiles, func(), error) {
	files := store.ImportFiles{Media: map[string]store.NewMedia{}, Skipped: map[string]string{}}
	ix := newZipIndex(zr, gedName)
	staged := map[*zip.File]*stagedFile{}
	byValue := map[string]*zip.File{}
	budget := s.maxGedcom() - int64(gedSize)

	discard := func() {
		for _, sf := range staged {
			if sf.staged != nil {
				sf.staged.Discard()
			}
		}
	}
	for _, value := range values {
		entry, reason := ix.resolve(value)
		if entry == nil {
			files.Skipped[value] = reason
			continue
		}
		if _, done := staged[entry]; done {
			byValue[value] = entry
			continue
		}
		sf, reason, err := s.stageEntry(entry, &budget)
		if err != nil {
			discard()
			return files, nil, err
		}
		if reason != "" {
			files.Skipped[value] = reason
			continue
		}
		staged[entry] = sf
		byValue[value] = entry
	}

	// Lock every hash, in one order so that two imports cannot wait on each
	// other, then move the files into the store.
	var shas []string
	seen := map[string]bool{}
	for _, sf := range staged {
		if !seen[sf.staged.SHA256] {
			seen[sf.staged.SHA256] = true
			shas = append(shas, sf.staged.SHA256)
		}
	}
	sort.Strings(shas)
	var unlocks []func()
	unlock := func() {
		for _, u := range unlocks {
			u()
		}
	}
	for _, sha := range shas {
		unlocks = append(unlocks, s.fileLocks.Lock(sha))
	}
	// removeUnused deletes the committed files that no medium uses, while
	// the locks are still held.
	removeUnused := func() {
		for _, sha := range shas {
			if used, err := s.Store.MediaFileUsed(context.WithoutCancel(ctx), sha); err != nil {
				s.Log.Warn("check media file", "sha", sha, "err", err)
			} else if !used {
				if err := s.Files.Remove(sha); err != nil {
					s.Log.Warn("remove media file", "sha", sha, "err", err)
				}
			}
		}
	}
	for _, sf := range staged {
		if err := sf.staged.Commit(); err != nil {
			discard()
			removeUnused()
			unlock()
			return files, nil, err
		}
	}
	for value, entry := range byValue {
		sf := staged[entry]
		in := store.NewMedia{
			SHA256: sf.staged.SHA256, Mime: sf.mime, Kind: string(sf.kind), Size: sf.staged.Size,
			OriginalName: sf.name, Orientation: 1,
		}
		if sf.kind == media.KindImage {
			s.readImageMeta(&in)
		}
		files.Media[value] = in
	}
	// Even a successful import can leave a file without a medium, when the
	// record that held its OBJE line was not imported, so finish always
	// removes the unused ones.
	finish := func() {
		defer unlock()
		removeUnused()
	}
	return files, finish, nil
}

// stageEntry writes one zip entry to a temporary file after the checks of
// an upload. A file that fails a check gives a reason and no error; an error
// means that the server could not store files.
func (s *Server) stageEntry(entry *zip.File, budget *int64) (*stagedFile, string, error) {
	if entry.UncompressedSize64 > uint64(s.MaxUpload) {
		return nil, skipTooLarge, nil
	}
	if entry.UncompressedSize64 > uint64(max(*budget, 0)) {
		return nil, skipBomb, nil
	}
	rc, err := entry.Open()
	if err != nil {
		s.Log.Warn("open zip entry", "file", entry.Name, "err", err)
		return nil, skipUnreadble, nil
	}
	defer rc.Close()
	buffered := bufio.NewReaderSize(rc, 4096)
	head, _ := buffered.Peek(512)
	name := path.Base(strings.ReplaceAll(entry.Name, `\`, "/"))
	mimeType, kind, ok := media.Detect(head, name)
	if !ok {
		return nil, skipType, nil
	}
	var check func(path string) error
	if kind == media.KindImage {
		check = func(path string) error {
			if err := media.CheckImageSize(path); errors.Is(err, media.ErrImageTooLarge) {
				return err
			}
			return nil
		}
	}
	// The declared size can lie; the limit of the read is what counts.
	limit := min(s.MaxUpload, max(*budget, 0))
	st, err := s.Files.Stage(buffered, limit, check)
	switch {
	case errors.Is(err, media.ErrImageTooLarge):
		return nil, skipPixels, nil
	case errors.Is(err, media.ErrTooLarge):
		if limit < s.MaxUpload {
			return nil, skipBomb, nil
		}
		return nil, skipTooLarge, nil
	case err != nil && isDiskError(err):
		return nil, "", err
	case err != nil:
		// The entry is damaged (a bad checksum or compressed data).
		s.Log.Warn("read zip entry", "file", entry.Name, "err", err)
		return nil, skipUnreadble, nil
	}
	*budget -= st.Size
	return &stagedFile{staged: st, mime: mimeType, kind: kind, name: name}, "", nil
}

// isDiskError tells a failure to write the temporary file from a damaged zip
// entry, which fails while it is read.
func isDiskError(err error) bool {
	var pe *fs.PathError
	return errors.As(err, &pe)
}
