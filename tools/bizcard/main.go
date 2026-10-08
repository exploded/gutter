// Command bizcard renders the Up The Spout business card as a
// print-ready PDF for Officeworks, and exports the logo lockups.
//
// The card follows the Officeworks spec: 90 × 55 mm trim with 5 mm bleed on
// every side (so each PDF page is 100 × 65 mm), text and logos at least 5 mm
// inside the trim, no crop marks. Page 1 is the front, page 2 the back.
//
//	go run ./tools/bizcard -name "Vin …" -phone "04xx xxx xxx" [-title Owner] [-email …] [-web upthespout.com.au] [-abn "…"] [-out tools/brand/out]
//	go run ./tools/bizcard -proof    # design review: placeholders allowed, every side stamped PROOF
//	go run ./tools/bizcard -logos    # logo-lockup.png, logo-lockup-dark.png, logo-mark-1024.png, logo-lockup.pdf
//
// It writes business-card.pdf plus business-card-front.png and -back.png
// previews (proof runs add a -proof suffix so they never overwrite a real
// card). Add -guides to draw the trim and safe area on the previews.
//
// Run it from the repo root: it reads the fonts from tools/brand/fonts and the
// mark from static/img/logo-mark.svg. It drives headless Chrome — the Windows
// default install, else $CHROME, else the usual macOS and Linux locations.
package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

//go:embed brand.tmpl card.tmpl logo.tmpl
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "*.tmpl"))

const (
	fontDir  = "tools/brand/fonts"
	markPath = "static/img/logo-mark.svg"
)

// Fixed card copy. Keep it to what the business can back up: no "fully
// insured", no "since 20xx", no ratings.
const (
	tagline  = "Clean gutters. No one on a ladder."
	services = "Gutters · Downpipes · Clean-up · Photo report"
	area     = "Melbourne's north-east · Fixed prices · Book online"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("bizcard: ")

	var c card
	flag.StringVar(&c.Name, "name", "", "name for the back of the card (required unless -proof)")
	flag.StringVar(&c.Phone, "phone", "", "phone number, as it should be printed (required unless -proof)")
	flag.StringVar(&c.Title, "title", "Owner", "role under the name; empty to omit")
	flag.StringVar(&c.Email, "email", "", "email address (optional)")
	flag.StringVar(&c.Web, "web", "upthespout.com.au", "website domain; the QR code points at https://<web>/book")
	flag.StringVar(&c.ABN, "abn", "", "ABN, printed small on the back (optional)")
	out := flag.String("out", "tools/brand/out", "output directory")
	proof := flag.Bool("proof", false, "design-review copy: allow empty fields and stamp PROOF across both sides")
	guides := flag.Bool("guides", false, "draw the bleed, trim and safe area on the PNG previews (never on the PDF)")
	logos := flag.Bool("logos", false, "export the logo lockups and mark instead of the card")
	flag.Parse()
	if flag.NArg() > 0 {
		log.Fatalf("unexpected arguments %q (quote values with spaces)", flag.Args())
	}

	c.Proof, c.Guides = *proof, *guides
	if !*logos {
		if err := c.prepare(); err != nil {
			fmt.Fprintln(os.Stderr, "bizcard:", err)
			os.Exit(2)
		}
	}
	assets, err := loadAssets()
	if err != nil {
		log.Fatal(err)
	}
	chrome, err := findChrome()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}

	if *logos {
		if err := exportLogos(chrome, assets, *out); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := makeCard(chrome, assets, c, *out); err != nil {
		log.Fatal(err)
	}
}

// assets are the on-disk brand files every page needs.
type assets struct {
	Fonts fontURLs
	Mark  template.HTML // static/img/logo-mark.svg, inlined
}

type fontURLs struct {
	Display, Sans400, Sans500, Sans800 template.URL
}

func loadAssets() (assets, error) {
	if fi, err := os.Stat(fontDir); err != nil || !fi.IsDir() {
		return assets{}, fmt.Errorf("%s not found: run this from the repo root (go run ./tools/bizcard …)", fontDir)
	}
	font := func(name string) (template.URL, error) {
		p := filepath.Join(fontDir, name)
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("missing font %s", p)
		}
		u, err := fileURL(p)
		return template.URL(u), err
	}
	var a assets
	var err error
	if a.Fonts.Display, err = font("LilitaOne-Regular.ttf"); err != nil {
		return a, err
	}
	if a.Fonts.Sans400, err = font("Figtree-Regular.ttf"); err != nil {
		return a, err
	}
	if a.Fonts.Sans500, err = font("Figtree-Medium.ttf"); err != nil {
		return a, err
	}
	if a.Fonts.Sans800, err = font("Figtree-ExtraBold.ttf"); err != nil {
		return a, err
	}
	svg, err := os.ReadFile(markPath)
	if err != nil {
		return a, fmt.Errorf("reading the mark: %w", err)
	}
	s := strings.TrimSpace(string(svg))
	if !strings.HasPrefix(s, "<svg") {
		return a, errors.New(markPath + " should start with <svg")
	}
	a.Mark = template.HTML(s)
	return a, nil
}

// fileURL turns a local path into an absolute file:/// URL Chrome can load
// (file:///C:/… on Windows, file:///home/… elsewhere).
func fileURL(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String(), nil
}

// renderTo executes the named template into dir/file and returns its URL.
func renderTo(dir, file, name string, data any) (string, error) {
	path := filepath.Join(dir, file)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if err := templates.ExecuteTemplate(f, name, data); err != nil {
		f.Close()
		return "", fmt.Errorf("rendering %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return fileURL(path)
}
