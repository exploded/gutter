package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// logoPage is the template data for logo.tmpl.
type logoPage struct {
	assets
	Variant      string // light | dark | mark-only
	LockupClass  string
	Size, X, Y   float64 // lockup em size and position, CSS px
	PDF          bool
	PageW, PageH float64 // PDF page size, CSS px
}

// The lockup is about 7.1 em wide, so at 162 px and 2× it comes out near
// 2300 px of artwork, about 2400 px with padding. The canvas leaves room on
// every side so the trim below never meets an edge.
const (
	logoEm             = 162
	logoScale          = 2
	logoCanvasW        = 1400
	logoCanvasH        = 440
	logoStageX         = 60
	logoStageY         = 80
	logoPadFrac        = 0.025 // padding around the trimmed artwork, as a fraction of its width
	markPreviewCSSSize = 512   // × logoScale = 1024 px
)

// exportLogos writes logo-lockup.png (for light backgrounds),
// logo-lockup-dark.png (for dark backgrounds) and logo-lockup-on-navy.png
// (for the brand navy itself, where the standard badge would vanish — the
// same treatment as the card front), all transparent and cropped to the same
// box; logo-mark-1024.png; and logo-lockup.pdf, a vector copy of the light
// lockup for sign writers.
func exportLogos(chrome string, a assets, outDir string) error {
	work, err := os.MkdirTemp("", "bizcard-logos-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	outPath := func(name string) (string, error) { return filepath.Abs(filepath.Join(outDir, name)) }
	transparent := "--default-background-color=00000000"

	var crop image.Rectangle
	for _, v := range []struct{ variant, class, file string }{
		{"light", "", "logo-lockup.png"},
		{"dark", "", "logo-lockup-dark.png"},
		{"dark", "on-ink", "logo-lockup-on-navy.png"},
	} {
		page, err := renderTo(work, v.file+".html", "logo.tmpl",
			logoPage{assets: a, Variant: v.variant, LockupClass: v.class, Size: logoEm, X: logoStageX, Y: logoStageY})
		if err != nil {
			return err
		}
		raw := filepath.Join(work, "raw-"+v.file)
		if err := screenshot(chrome, work, page, raw, logoCanvasW, logoCanvasH, logoScale, transparent); err != nil {
			return err
		}
		img, err := readPNG(raw)
		if err != nil {
			return err
		}
		if crop.Empty() {
			bb := opaqueBounds(img)
			if bb.Empty() {
				return fmt.Errorf("%s rendered blank (is the background transparent and the mark loading?)", v.file)
			}
			edge := img.Bounds().Inset(2)
			if !bb.In(edge) {
				return fmt.Errorf("the lockup touches the edge of the %v canvas; make logoCanvasW/H bigger", img.Bounds().Size())
			}
			pad := int(float64(bb.Dx())*logoPadFrac + 0.5)
			crop = bb.Inset(-pad).Intersect(img.Bounds())
		}
		dst, err := outPath(v.file)
		if err != nil {
			return err
		}
		if err := writePNG(dst, cropRGBA(img, crop)); err != nil {
			return err
		}
		fmt.Printf("wrote %s (%d×%d)\n", dst, crop.Dx(), crop.Dy())
	}

	// The badge on its own, rendered straight from the SVG.
	page, err := renderTo(work, "mark.html", "logo.tmpl", logoPage{assets: a, Variant: "mark-only"})
	if err != nil {
		return err
	}
	dst, err := outPath("logo-mark-1024.png")
	if err != nil {
		return err
	}
	if err := screenshot(chrome, work, page, dst, markPreviewCSSSize, markPreviewCSSSize, logoScale, transparent); err != nil {
		return err
	}
	fmt.Println("wrote", dst)

	// Vector PDF of the light lockup, cropped to the same box as the PNG.
	// Chrome snaps the page size to its own grid, so print it oversize and let
	// exactPages crop it back to the box (CSS px are 0.75 pt).
	s := float64(logoScale)
	boxW, boxH := float64(crop.Dx())/s, float64(crop.Dy())/s
	page, err = renderTo(work, "lockup-pdf.html", "logo.tmpl", logoPage{
		assets: a, Variant: "light", Size: logoEm, PDF: true,
		PageW: boxW + 8, PageH: boxH + 8,
		X: logoStageX - float64(crop.Min.X)/s, Y: logoStageY - float64(crop.Min.Y)/s,
	})
	if err != nil {
		return err
	}
	if dst, err = outPath("logo-lockup.pdf"); err != nil {
		return err
	}
	if err := printPDF(chrome, work, page, dst); err != nil {
		return err
	}
	if err := exactPages(dst, boxW*0.75, boxH*0.75, 0); err != nil {
		return err
	}
	fmt.Println("wrote", dst)
	return nil
}

func readPNG(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	img := image.NewNRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	return img, nil
}

// opaqueBounds is the smallest rectangle holding every non-transparent pixel.
func opaqueBounds(img *image.NRGBA) image.Rectangle {
	b := img.Bounds()
	r := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			r.Min.X, r.Min.Y = min(r.Min.X, x), min(r.Min.Y, y)
			r.Max.X, r.Max.Y = max(r.Max.X, x+1), max(r.Max.Y, y+1)
		}
	}
	if r.Min.X >= r.Max.X {
		return image.Rectangle{}
	}
	return r
}

func cropRGBA(img *image.NRGBA, r image.Rectangle) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
