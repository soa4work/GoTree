package api

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/praetorianer777/gotree/internal/media"
	"github.com/praetorianer777/gotree/internal/store"
)

func (s *Server) mediaRoutes(r chi.Router) {
	r.Route("/media", func(r chi.Router) {
		r.Get("/", s.listMedia)
		r.Post("/", s.uploadMedia)
		r.Get("/{id}", s.getMedia)
		r.Put("/{id}", s.updateMedia)
		r.Delete("/{id}", s.deleteMedia)
		r.Get("/{id}/file", s.mediaFile)
		r.Get("/{id}/thumb", s.mediaThumb)
		r.Post("/{id}/links", s.linkMedia)
		r.Delete("/{id}/links/{type}/{entityID}", s.unlinkMedia)
		r.Post("/{id}/regions", s.addRegion)
		r.Put("/regions/{regionID}", s.updateRegion)
		r.Delete("/regions/{regionID}", s.deleteRegion)
	})
	r.Put("/persons/{id}/portrait", s.setPortrait)
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var entityID int64
	if t := q.Get("entityType"); t != "" {
		var err error
		if entityID, err = strconv.ParseInt(q.Get("entityId"), 10, 64); err != nil {
			writeError(w, http.StatusBadRequest, "entityId is required with entityType")
			return
		}
	}
	list, err := s.Store.ListMedia(r.Context(), actorFrom(r), q.Get("entityType"), entityID, queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	s.respond(w, http.StatusOK, list, err)
}

// uploadMedia streams one file ("file" part of a multipart form) to disk,
// identifies it from its content and reads its photo metadata. An
// optional ?entityType=&entityId= links it in the same step.
func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request) {
	var link *store.MediaLinkInput
	if t := r.URL.Query().Get("entityType"); t != "" {
		id, err := strconv.ParseInt(r.URL.Query().Get("entityId"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "entityId is required with entityType")
			return
		}
		link = &store.MediaLinkInput{EntityType: t, EntityID: id}
	}

	// The multipart framing adds a little to the file itself.
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxUpload+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected a multipart upload")
		return
	}
	var part *multipart.Part
	for {
		part, err = mr.NextPart()
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "no file in the upload")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid upload")
			return
		}
		if part.FormName() == "file" {
			break
		}
	}
	defer part.Close()

	buffered := bufio.NewReaderSize(part, 4096)
	head, _ := buffered.Peek(512)
	mimeType, kind, ok := media.Detect(head, part.FileName())
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "this kind of file is not supported; use JPEG, PNG, GIF, WebP, PDF, MP3, M4A, OGG, WAV, MP4 or WebM")
		return
	}
	stored, err := s.Files.Save(buffered, s.MaxUpload)
	if errors.Is(err, media.ErrTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("the file is larger than %d MB", s.MaxUpload>>20))
		return
	}
	if err != nil {
		s.Log.Error("store upload", "err", err)
		writeError(w, http.StatusInternalServerError, "could not store the file")
		return
	}

	in := store.NewMedia{
		SHA256: stored.SHA256, Mime: mimeType, Kind: string(kind), Size: stored.Size,
		OriginalName: filepath.Base(part.FileName()), Orientation: 1,
	}
	if kind == media.KindImage {
		if path, err := s.Files.Path(stored.SHA256); err == nil && errors.Is(media.CheckImageSize(path), media.ErrImageTooLarge) {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("the image is larger than %d megapixels", media.MaxImagePixels/1_000_000))
			return
		}
		s.readImageMeta(&in)
	}
	m, created, err := s.Store.CreateMedia(r.Context(), actorFrom(r), in, link)
	if err != nil {
		// The file may now be unused; it is small to keep and removing it
		// here could race with an identical upload, so it stays.
		writeStoreError(w, s.Log, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, m)
}

func (s *Server) readImageMeta(in *store.NewMedia) {
	path, err := s.Files.Path(in.SHA256)
	if err != nil {
		return
	}
	if in.Mime == "image/jpeg" {
		if f, err := os.Open(path); err == nil {
			meta := media.ReadMeta(f)
			f.Close()
			in.Orientation, in.TakenAt, in.Lat, in.Lng = meta.Orientation, meta.TakenAt, meta.Lat, meta.Lng
			for _, reg := range meta.Regions {
				in.Regions = append(in.Regions, store.RegionInput{Name: reg.Name, X: reg.X, Y: reg.Y, W: reg.W, H: reg.H})
			}
		}
	}
	if w, h, err := media.DisplaySize(path, in.Orientation); err == nil {
		in.Width, in.Height = &w, &h
	}
}

func (s *Server) getMedia(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		m, err := s.Store.GetMedia(r.Context(), actorFrom(r), id)
		s.respond(w, http.StatusOK, m, err)
	}
}

func (s *Server) updateMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	var in store.MediaUpdate
	if ok && decode(w, r, &in) {
		m, err := s.Store.UpdateMedia(r.Context(), actorFrom(r), id, in)
		s.respond(w, http.StatusOK, m, err)
	}
}

func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	sha, orphaned, err := s.Store.DeleteMedia(r.Context(), actorFrom(r), id)
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	if orphaned {
		if err := s.Files.Remove(sha); err != nil {
			s.Log.Warn("remove media file", "sha", sha, "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// mediaFile serves the original. Content-addressed files never change, so
// they may be cached for good; range requests let players seek.
func (s *Server) mediaFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	m, err := s.Store.GetMedia(r.Context(), actorFrom(r), id)
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	path, err := s.Files.Path(m.SHA256)
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		s.Log.Error("open media file", "id", id, "err", err)
		writeError(w, http.StatusNotFound, "the file is missing on the server")
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", m.Mime)
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	// Only types that cannot run scripts are accepted, but in case a
	// browser thinks otherwise the file gets no powers. Chrome's PDF
	// viewer does not run in a sandboxed document, so PDFs skip that.
	if m.Mime == "application/pdf" {
		h.Set("Content-Security-Policy", "default-src 'none'; object-src 'self'; style-src 'unsafe-inline'")
	} else {
		h.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self'; media-src 'self'")
	}
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	h.Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, safeFilename(m)))
	http.ServeContent(w, r, "", parseTime(m.CreatedAt), f)
}

func safeFilename(m store.Media) string {
	name := m.OriginalName
	if name == "" {
		name = fmt.Sprintf("media-%d", m.ID)
	}
	out := []rune{}
	for _, c := range name {
		if c < 32 || c == '"' || c == '\\' || c == 127 {
			c = '_'
		}
		out = append(out, c)
	}
	return string(out)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func (s *Server) mediaThumb(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	a := actorFrom(r)
	m, err := s.Store.GetMedia(r.Context(), a, id)
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	if m.Kind != string(media.KindImage) {
		writeError(w, http.StatusNotFound, "only photos have thumbnails")
		return
	}
	size := queryInt(r, "size", 256)
	var crop *media.Crop
	if rid := queryInt(r, "region", 0); rid > 0 {
		reg, err := s.Store.Region(r.Context(), a, id, int64(rid))
		if err != nil {
			writeStoreError(w, s.Log, err)
			return
		}
		crop = &media.Crop{ID: reg.ID, X: reg.X, Y: reg.Y, W: reg.W, H: reg.H}
	}
	path, err := s.Files.Thumb(m.SHA256, size, m.Orientation, crop)
	if err != nil {
		s.Log.Warn("thumbnail", "id", id, "err", err)
		if errors.Is(err, media.ErrImageTooLarge) {
			writeError(w, http.StatusUnprocessableEntity, "the image is too large to render a thumbnail")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "could not render a thumbnail")
		return
	}
	// The URL includes the region id but not its box, which can move, so
	// cropped thumbnails are revalidated.
	if crop == nil {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, path)
}

func (s *Server) linkMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	var in store.MediaLinkInput
	if ok && decode(w, r, &in) {
		m, err := s.Store.LinkMedia(r.Context(), actorFrom(r), id, in)
		s.respond(w, http.StatusOK, m, err)
	}
}

func (s *Server) unlinkMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	entityID, ok := pathID(w, r, "entityID")
	if !ok {
		return
	}
	s.noContent(w, s.Store.UnlinkMedia(r.Context(), actorFrom(r), id, chi.URLParam(r, "type"), entityID))
}

func (s *Server) addRegion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	var in store.RegionInput
	if ok && decode(w, r, &in) {
		m, err := s.Store.AddRegion(r.Context(), actorFrom(r), id, in)
		s.respond(w, http.StatusCreated, m, err)
	}
}

func (s *Server) updateRegion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "regionID")
	var in store.RegionInput
	if ok && decode(w, r, &in) {
		m, err := s.Store.UpdateRegion(r.Context(), actorFrom(r), id, in)
		s.respond(w, http.StatusOK, m, err)
	}
}

func (s *Server) deleteRegion(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "regionID"); ok {
		m, err := s.Store.DeleteRegion(r.Context(), actorFrom(r), id)
		s.respond(w, http.StatusOK, m, err)
	}
}

func (s *Server) setPortrait(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	var in store.PortraitInput
	if ok && decode(w, r, &in) {
		p, err := s.Store.SetPortrait(r.Context(), actorFrom(r), id, in)
		s.respond(w, http.StatusOK, p, err)
	}
}
