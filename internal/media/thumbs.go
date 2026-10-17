package media

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"

	// Decoders for the accepted image types.
	_ "image/gif"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// ThumbSizes are the sizes thumbnails are rendered in (longest side).
var ThumbSizes = []int{128, 256, 512, 1024}

// MaxImagePixels is the largest image (width times height) that is decoded.
// A decoded image takes four bytes per pixel and rotating it needs a second
// copy (8 bytes per pixel for 16-bit PNGs), so a small file declaring huge
// dimensions could exhaust memory.
const MaxImagePixels = 50_000_000

// ErrImageTooLarge is returned for images with more than MaxImagePixels.
var ErrImageTooLarge = errors.New("image has too many pixels")

// thumbSlots bounds how many thumbnails are rendered at once. Each holds a
// fully decoded image and its rotated copy, up to about 600 MB at the pixel
// limit, so the bound stays low even on machines with many CPUs.
var thumbSlots = make(chan struct{}, min(runtime.NumCPU(), 2))

// CheckImageSize reads an image's dimensions from its header, without
// decoding it, and returns ErrImageTooLarge above MaxImagePixels.
func CheckImageSize(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("invalid image size %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Width > MaxImagePixels/cfg.Height {
		return ErrImageTooLarge
	}
	return nil
}

// Crop selects part of the displayed image, relative 0-1.
type Crop struct {
	ID         int64
	X, Y, W, H float64
}

// Thumb returns the path of a JPEG thumbnail of an image, rendering and
// caching it on first use. The image is rotated by its EXIF orientation
// and optionally cropped first.
func (f Files) Thumb(sha string, size, orientation int, crop *Crop) (string, error) {
	if !validSize(size) {
		return "", fmt.Errorf("unsupported size %d", size)
	}
	src, err := f.Path(sha)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%d-o%d", sha, size, orientation)
	if crop != nil {
		name += fmt.Sprintf("-r%d-%.4f-%.4f-%.4f-%.4f", crop.ID, crop.X, crop.Y, crop.W, crop.H)
	}
	dst := filepath.Join(f.Root, "thumbs", sha[:2], name+".jpg")
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}

	// Wait for a free slot, then look again: another request may have
	// rendered this thumbnail meanwhile.
	thumbSlots <- struct{}{}
	defer func() { <-thumbSlots }()
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	if err := CheckImageSize(src); err != nil {
		return "", err
	}

	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(in)
	in.Close()
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	img = Orient(img, orientation)
	if crop != nil {
		b := img.Bounds()
		r := image.Rect(
			b.Min.X+int(crop.X*float64(b.Dx())), b.Min.Y+int(crop.Y*float64(b.Dy())),
			b.Min.X+int((crop.X+crop.W)*float64(b.Dx())), b.Min.Y+int((crop.Y+crop.H)*float64(b.Dy())),
		).Intersect(b)
		if r.Empty() {
			return "", errors.New("empty crop")
		}
		img = subImage(img, r)
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > size || h > size {
		if w >= h {
			w, h = size, max(1, h*size/w)
		} else {
			w, h = max(1, w*size/h), size
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(out, out.Bounds(), img, b, draw.Src, nil)

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "thumb-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if err := jpeg.Encode(tmp, out, &jpeg.Options{Quality: 82}); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	// Two requests may render the same thumbnail at once; rename makes the
	// last one win without either reading a half-written file.
	return dst, os.Rename(tmp.Name(), dst)
}

func validSize(size int) bool {
	for _, s := range ThumbSizes {
		if s == size {
			return true
		}
	}
	return false
}

func subImage(img image.Image, r image.Rectangle) image.Image {
	if s, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return s.SubImage(r)
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Copy(out, image.Point{}, img, r, draw.Src, nil)
	return out
}

// Orient applies an EXIF orientation, so the image looks the way the
// camera was held.
func Orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := w, h
	if o >= 5 {
		ow, oh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2: // mirrored
				dx, dy = w-1-x, y
			case 3: // rotated 180°
				dx, dy = w-1-x, h-1-y
			case 4: // mirrored vertically
				dx, dy = x, h-1-y
			case 5: // transposed
				dx, dy = y, x
			case 6: // rotated 90° clockwise
				dx, dy = h-1-y, x
			case 7: // transversed
				dx, dy = h-1-y, w-1-x
			case 8: // rotated 90° counter-clockwise
				dx, dy = y, w-1-x
			}
			out.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// DisplaySize returns an image's width and height as displayed, i.e.
// after its orientation is applied.
func DisplaySize(path string, orientation int) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	if orientation >= 5 {
		return cfg.Height, cfg.Width, nil
	}
	return cfg.Width, cfg.Height, nil
}
