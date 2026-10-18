package media

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// gpsJPEG is a JPEG with orientation 6, a capture time and a GPS position.
func gpsJPEG(t *testing.T) []byte {
	t.Helper()
	exif := buildEXIF(binary.LittleEndian, 6, "2020:05:06 07:08:09", 'N', [3]float64{51, 20, 24}, 'E', [3]float64{12, 22, 12})
	return withSegments(encodeJPEG(t, 64, 32), exif)
}

func saved(t *testing.T, f Files, data []byte) string {
	t.Helper()
	s, err := f.Save(bytes.NewReader(data), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return s.SHA256
}

func readMetaFile(t *testing.T, path string) Meta {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	return ReadMeta(fh)
}

func TestWithoutLocationJPEG(t *testing.T) {
	f := Files{Root: t.TempDir()}
	orig := gpsJPEG(t)
	sha := saved(t, f, orig)
	origPath, _ := f.Path(sha)
	if m := readMetaFile(t, origPath); m.Lat == nil || m.Lng == nil {
		t.Fatal("test image has no position")
	}

	clean, err := f.WithoutLocation(sha, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if clean == origPath {
		t.Fatal("original was served")
	}
	m := readMetaFile(t, clean)
	if m.Lat != nil || m.Lng != nil {
		t.Errorf("position survived: %v %v", m.Lat, m.Lng)
	}
	if m.Orientation != 6 || m.TakenAt != "2020-05-06 07:08:09" {
		t.Errorf("orientation and time lost: %+v", m)
	}
	got, _ := os.ReadFile(clean)
	if len(got) != len(orig) {
		t.Errorf("size %d, want %d", len(got), len(orig))
	}
	if _, _, err := image.Decode(bytes.NewReader(got)); err != nil {
		t.Errorf("clean copy does not decode: %v", err)
	}
	if after, _ := os.ReadFile(origPath); !bytes.Equal(after, orig) {
		t.Error("the original changed")
	}
	// The second call serves the cached copy.
	if again, _ := f.WithoutLocation(sha, "image/jpeg"); again != clean {
		t.Errorf("second call: %s", again)
	}
}

func TestWithoutLocationBigEndianAndIFD1(t *testing.T) {
	// A big-endian block works the same.
	exif := buildEXIF(binary.BigEndian, 3, "", 'S', [3]float64{1, 2, 3}, 'W', [3]float64{4, 5, 6})
	f := Files{Root: t.TempDir()}
	sha := saved(t, f, withSegments(encodeJPEG(t, 16, 16), exif))
	clean, err := f.WithoutLocation(sha, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if m := readMetaFile(t, clean); m.Lat != nil || m.Orientation != 3 {
		t.Errorf("meta: %+v", m)
	}
}

func TestWithoutLocationXMP(t *testing.T) {
	xmp := strings.Replace(xmpAttributes, "</rdf:RDF>",
		`<rdf:Description exif:GPSLatitude="51,20.4N" xmlns:exif="e"><exif:GPSLongitude>12,22.2E</exif:GPSLongitude></rdf:Description></rdf:RDF>`, 1)
	f := Files{Root: t.TempDir()}
	sha := saved(t, f, withSegments(encodeJPEG(t, 16, 16), xmpSegment(xmp)))
	origPath, _ := f.Path(sha)
	want := readMetaFile(t, origPath).Regions

	clean, err := f.WithoutLocation(sha, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if clean == origPath {
		t.Fatal("original was served")
	}
	got, _ := os.ReadFile(clean)
	for _, secret := range []string{"51,20.4N", "12,22.2E"} {
		if bytes.Contains(got, []byte(secret)) {
			t.Errorf("%s survived", secret)
		}
	}
	if regions := readMetaFile(t, clean).Regions; len(want) == 0 || !reflect.DeepEqual(regions, want) {
		t.Errorf("regions: %v, want %v", regions, want)
	}
}

func TestBlankXMPLocationVariants(t *testing.T) {
	in := `<a drone-dji:GpsLatitude='+51.3' tiff:Make="Cam"><exif:GPSAltitude>10</exif:GPSAltitude><exif:GPSTimeStamp/>` +
		`<exif:GPSLatitude><rdf:Seq><rdf:li>51</rdf:li></rdf:Seq></exif:GPSLatitude></a>`
	b := []byte(in)
	if !blankXMPLocation(b) {
		t.Fatal("nothing blanked")
	}
	out := string(b)
	if len(out) != len(in) {
		t.Errorf("length changed: %d, want %d", len(out), len(in))
	}
	for _, secret := range []string{"+51.3", ">10<", ">51<"} {
		if strings.Contains(out, secret) {
			t.Errorf("%s survived: %s", secret, out)
		}
	}
	if !strings.Contains(out, `tiff:Make="Cam"`) {
		t.Errorf("other properties changed: %s", out)
	}
	if blankXMPLocation(b) {
		t.Error("second pass reports a change")
	}
}

func TestWithoutLocationNoGPS(t *testing.T) {
	f := Files{Root: t.TempDir()}
	jpg := withSegments(encodeJPEG(t, 16, 16), buildEXIF(binary.LittleEndian, 1, "2020:05:06 07:08:09", 0, [3]float64{}, 0, [3]float64{}))
	sha := saved(t, f, jpg)
	origPath, _ := f.Path(sha)
	p, err := f.WithoutLocation(sha, "image/jpeg")
	if err != nil || p != origPath {
		t.Fatalf("path %s, err %v", p, err)
	}
	marker := filepath.Join(f.Root, "clean", sha[:2], sha+"-"+cleanVersion+".same")
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("no marker: %v", err)
	}
	// Remove the temp dir contents to see that nothing is copied again.
	tmp, _ := os.ReadDir(filepath.Join(f.Root, "tmp"))
	if len(tmp) != 0 {
		t.Errorf("temporary files left: %d", len(tmp))
	}
	if p, _ := f.WithoutLocation(sha, "image/jpeg"); p != origPath {
		t.Errorf("second call: %s", p)
	}
}

func TestWithoutLocationMalformedGPS(t *testing.T) {
	exif := buildEXIF(binary.LittleEndian, 6, "", 'N', [3]float64{1, 2, 3}, 'E', [3]float64{4, 5, 6})
	// Point the GPS IFD far outside the block: payload is "Exif\0\0" + TIFF,
	// IFD0 has Orientation (tag 0x0112) then GPS (0x8825).
	tiffAt := 4 + 6
	tiffData := exif[tiffAt:]
	patched := false
	for i := 0; i+12 <= len(tiffData); i++ {
		if tiffData[i] == 0x25 && tiffData[i+1] == 0x88 && tiffData[i+2] == 4 { // tag 0x8825, LONG
			binary.LittleEndian.PutUint32(tiffData[i+8:], 0x7fffff00)
			patched = true
			break
		}
	}
	if !patched {
		t.Fatal("GPS entry not found")
	}
	f := Files{Root: t.TempDir()}
	sha := saved(t, f, withSegments(encodeJPEG(t, 16, 16), exif))
	clean, err := f.WithoutLocation(sha, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if m := readMetaFile(t, clean); m.Orientation != 1 || m.Lat != nil {
		t.Errorf("TIFF data not cleared: %+v", m)
	}
	got, _ := os.ReadFile(clean)
	if bytes.Contains(got, []byte("II*")) {
		t.Error("TIFF header survived")
	}
}

func pngChunk(typ string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, typ...)
	out = append(out, data...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(append([]byte(typ), data...)))
}

func TestWithoutLocationPNG(t *testing.T) {
	var base bytes.Buffer
	if err := png.Encode(&base, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	// Insert eXIf after the signature and IHDR chunk (8 + 25 bytes).
	exif := buildEXIF(binary.LittleEndian, 1, "", 'N', [3]float64{51, 20, 24}, 'E', [3]float64{12, 22, 12})
	tiffData := exif[4+6:]
	gps := binary.LittleEndian.AppendUint32(nil, 51000) // 51 degrees as a rational numerator
	if !bytes.Contains(tiffData, gps) {
		t.Fatal("test GPS data not found")
	}
	itxt := append([]byte("XML:com.adobe.xmp\x00\x00\x00\x00\x00"), `<a exif:GPSLatitude="51,20N"/>`...)
	raw := base.Bytes()
	data := append(append(append([]byte{}, raw[:33]...), pngChunk("eXIf", tiffData)...), pngChunk("iTXt", itxt)...)
	data = append(data, raw[33:]...)

	f := Files{Root: t.TempDir()}
	sha := saved(t, f, data)
	clean, err := f.WithoutLocation(sha, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(clean)
	if len(got) != len(data) {
		t.Errorf("size %d, want %d", len(got), len(data))
	}
	if _, err := png.Decode(bytes.NewReader(got)); err != nil {
		t.Errorf("clean PNG does not decode (CRC?): %v", err)
	}
	if bytes.Contains(got, gps) || bytes.Contains(got, []byte("51,20N")) {
		t.Error("position survived")
	}
}

func TestWithoutLocationWebP(t *testing.T) {
	exif := buildEXIF(binary.LittleEndian, 1, "", 'N', [3]float64{51, 20, 24}, 'E', [3]float64{12, 22, 12})
	tiffData := exif[4+6:]
	gps := binary.LittleEndian.AppendUint32(nil, 51000)
	chunk := func(typ string, d []byte) []byte {
		out := append([]byte(typ), binary.LittleEndian.AppendUint32(nil, uint32(len(d)))...)
		out = append(out, d...)
		if len(d)%2 == 1 {
			out = append(out, 0)
		}
		return out
	}
	body := append([]byte("WEBP"), chunk("VP8X", make([]byte, 10))...)
	body = append(body, chunk("EXIF", tiffData)...)
	body = append(body, chunk("XMP ", []byte(`<a exif:GPSLongitude="12,22.2E"/>`))...)
	data := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
	data = append(data, body...)

	f := Files{Root: t.TempDir()}
	sha := saved(t, f, data)
	clean, err := f.WithoutLocation(sha, "image/webp")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(clean)
	if len(got) != len(data) || bytes.Contains(got, gps) || bytes.Contains(got, []byte("12,22.2E")) {
		t.Errorf("webp: len %d (want %d), position left: %v", len(got), len(data), bytes.Contains(got, gps))
	}
}

func TestWithoutLocationOtherTypes(t *testing.T) {
	f := Files{Root: t.TempDir()}
	for _, mime := range []string{"application/pdf", "audio/mpeg"} {
		sha := saved(t, f, []byte("data of "+mime))
		want, _ := f.Path(sha)
		if got, err := f.WithoutLocation(sha, mime); err != nil || got != want {
			t.Errorf("%s: %s, %v", mime, got, err)
		}
	}
	if _, err := f.WithoutLocation("nonsense", "image/jpeg"); err == nil {
		t.Error("invalid hash accepted")
	}
}

func TestRemoveDeletesCleanCopies(t *testing.T) {
	f := Files{Root: t.TempDir()}
	sha := saved(t, f, gpsJPEG(t))
	if _, err := f.WithoutLocation(sha, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if err := f.Remove(sha); err != nil {
		t.Fatal(err)
	}
	left, _ := filepath.Glob(filepath.Join(f.Root, "clean", "*", "*"))
	if len(left) != 0 {
		t.Errorf("left: %v", left)
	}
}

func TestThumbHasNoMetadata(t *testing.T) {
	f := Files{Root: t.TempDir()}
	sha := saved(t, f, gpsJPEG(t))
	thumb, err := f.Thumb(sha, ThumbSizes[0], 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(thumb)
	if m := ReadMeta(bytes.NewReader(got)); m.Lat != nil || m.Lng != nil {
		t.Errorf("thumbnail has a position: %+v", m)
	}
	if bytes.Contains(got, []byte("Exif\x00\x00")) {
		t.Error("thumbnail has an EXIF block")
	}
}

func TestHashLocks(t *testing.T) {
	var l HashLocks
	var inside, maxInside atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := l.Lock("same")
			n := inside.Add(1)
			for {
				old := maxInside.Load()
				if n <= old || maxInside.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			inside.Add(-1)
			unlock()
			unlock() // releasing twice is harmless
		}()
	}
	wg.Wait()
	if maxInside.Load() != 1 {
		t.Errorf("%d holders at once", maxInside.Load())
	}

	// Different hashes do not wait for each other.
	unlockA := l.Lock("a")
	done := make(chan struct{})
	go func() { l.Lock("b")(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("b blocked by a")
	}
	unlockA()
	if len(l.held) != 0 {
		t.Errorf("%d entries left", len(l.held))
	}
}

func TestStageCommit(t *testing.T) {
	f := Files{Root: t.TempDir()}
	s, err := f.Stage(strings.NewReader("hello"), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := f.Path(s.SHA256); fileExists(p) {
		t.Fatal("stored before Commit")
	}
	if err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	s.Discard() // no-op after Commit
	if p, _ := f.Path(s.SHA256); !fileExists(p) {
		t.Fatal("not stored")
	}
	if err := s.Commit(); err != nil {
		t.Errorf("second Commit: %v", err)
	}

	// An identical upload is dropped and leaves nothing behind.
	again, err := f.Stage(strings.NewReader("hello"), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := again.Commit(); err != nil {
		t.Fatal(err)
	}
	// Discard without Commit removes the temporary file.
	d, err := f.Stage(strings.NewReader("other"), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	d.Discard()
	if left, _ := os.ReadDir(filepath.Join(f.Root, "tmp")); len(left) != 0 {
		t.Errorf("%d temporary files left", len(left))
	}
	if p, _ := f.Path(d.SHA256); fileExists(p) {
		t.Error("discarded file stored")
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// TestBlankTIFFGPSIndirectPointer: a GPS pointer stored as two LONGs makes
// ReadMeta follow one more offset; such a block is cleared, not misread.
func TestBlankTIFFGPSIndirectPointer(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		exif := buildEXIF(order, 6, "", 'N', [3]float64{51, 20, 24}, 'E', [3]float64{12, 22, 12})
		b := append([]byte{}, exif[4+6:]...)
		for i := 8; i+12 <= len(b); i++ {
			if order.Uint16(b[i:]) == 0x8825 && order.Uint16(b[i+2:]) == 4 {
				at := len(b)
				b = append(b, make([]byte, 8)...)
				order.PutUint32(b[at:], order.Uint32(b[i+8:]))
				order.PutUint32(b[i+4:], 2)
				order.PutUint32(b[i+8:], uint32(at))
				break
			}
		}
		var before, after Meta
		parseTIFF(b, &before)
		if before.Lat == nil {
			t.Fatalf("%v: test block has no position", order)
		}
		blankTIFFGPS(b)
		if parseTIFF(b, &after); after.Lat != nil {
			t.Errorf("%v: position survived", order)
		}
	}
}

// TestWithoutLocationTruncatedSegment: a file that ends inside its EXIF
// segment keeps none of the bytes that are there.
func TestWithoutLocationTruncatedSegment(t *testing.T) {
	exif := buildEXIF(binary.LittleEndian, 1, "", 'N', [3]float64{51, 20, 24}, 'E', [3]float64{12, 22, 12})
	gps := binary.LittleEndian.AppendUint32(nil, 51000)
	data := append([]byte{0xFF, 0xD8}, exif[:len(exif)-4]...)
	if !bytes.Contains(data, gps) {
		t.Fatal("test GPS data not found")
	}
	f := Files{Root: t.TempDir()}
	clean, err := f.WithoutLocation(saved(t, f, data), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(clean); len(got) != len(data) || bytes.Contains(got, gps) {
		t.Errorf("truncated segment kept its position (%d bytes)", len(got))
	}
}
