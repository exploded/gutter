package main

import (
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/skip2/go-qrcode"
)

// card is what the user asked to print.
type card struct {
	Name, Phone, Title, Email, Web, ABN string
	Proof, Guides                       bool
}

// prepare tidies the fields and enforces the one rule that matters most: a
// print PDF never goes out with placeholder or missing contact details.
func (c *card) prepare() error {
	for _, f := range []*string{&c.Name, &c.Phone, &c.Title, &c.Email, &c.Web, &c.ABN} {
		*f = strings.Join(strings.Fields(*f), " ")
	}
	c.Web = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(c.Web), "https://"), "http://"), "/")
	if c.Web == "" || strings.ContainsAny(c.Web, "/ ") {
		return fmt.Errorf("-web should be a bare domain like warrandytegutters.com.au, not %q", c.Web)
	}
	if c.Email != "" && !strings.Contains(c.Email, "@") {
		return fmt.Errorf("-email %q doesn't look like an email address", c.Email)
	}
	if c.Proof {
		if c.Name == "" {
			c.Name = "Firstname Lastname"
		}
		if c.Phone == "" {
			c.Phone = "04XX XXX XXX"
		}
		return nil
	}
	var missing []string
	if c.Name == "" {
		missing = append(missing, "-name")
	}
	if c.Phone == "" {
		missing = append(missing, "-phone")
	}
	if len(missing) > 0 {
		return fmt.Errorf("refusing to make a print PDF without %s: placeholder contact details must never reach the printer.\n"+
			"Pass the real details, or add -proof for a design-review copy stamped PROOF", strings.Join(missing, " and "))
	}
	digits := 0
	for _, r := range c.Phone {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	if digits < 8 {
		return fmt.Errorf("-phone %q doesn't look like a phone number", c.Phone)
	}
	return nil
}

// cardPage is the template data for card.tmpl.
type cardPage struct {
	assets
	Mode                                string // print | front | back
	Proof, Guides                       bool
	LockupClass                         string
	Name, Title, Phone, Email, Web, ABN string
	Tagline, Services, Area             string
	QR                                  template.HTML
	QRSymbolMM, QRTotalMM, QRQuietMM    string // CSS lengths in mm
}

// Preview sizes in CSS px: the 90 × 55 mm trim, or with -guides the whole
// 100 × 65 mm page. Rendered at 3× (about 290 dpi at card size).
const (
	trimW, trimH   = 340, 208
	pageW, pageH   = 378, 246
	previewScale   = 3
	qrSymbolMM     = 18.6 // printed width of the QR symbol itself (spec: at least 18 mm)
	qrQuietModules = 2
)

func makeCard(chrome string, a assets, c card, outDir string) error {
	work, err := os.MkdirTemp("", "bizcard-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	target := "https://" + c.Web + "/book"
	qr, modules, err := qrSVG(target)
	if err != nil {
		return err
	}
	module := qrSymbolMM / float64(modules)
	if module < 0.45 {
		return fmt.Errorf("QR modules would be %.2f mm, too small to scan reliably; shorten -web", module)
	}
	mm := func(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }

	p := cardPage{
		assets: a, Proof: c.Proof, Guides: c.Guides, LockupClass: "on-green",
		Name: c.Name, Title: c.Title, Phone: c.Phone, Email: c.Email, Web: c.Web, ABN: c.ABN,
		Tagline: tagline, Services: services, Area: area, QR: qr,
		QRSymbolMM: mm(qrSymbolMM),
		QRTotalMM:  mm(module * float64(modules+2*qrQuietModules)),
		QRQuietMM:  mm(module * qrQuietModules),
	}
	base := "business-card"
	if c.Proof {
		base += "-proof"
	}
	outPath := func(name string) (string, error) { return filepath.Abs(filepath.Join(outDir, name)) }

	// Print: check the layout first, so a bad layout never leaves a PDF behind.
	p.Mode = "print"
	page, err := renderTo(work, "card.html", "card.tmpl", p)
	if err != nil {
		return err
	}
	if err := layoutCheck(chrome, work, page); err != nil {
		return fmt.Errorf("layout check failed, nothing written:\n  %v", err)
	}
	pdf, err := outPath(base + ".pdf")
	if err != nil {
		return err
	}
	if err := printPDF(chrome, work, page, pdf); err != nil {
		return err
	}
	if err := exactPages(pdf, 100*ptPerMM, 65*ptPerMM, 5*ptPerMM); err != nil {
		os.Remove(pdf)
		return err
	}
	if err := checkPDF(pdf); err != nil {
		os.Remove(pdf)
		return err
	}
	written := []string{pdf}

	for _, side := range []string{"front", "back"} {
		p.Mode = side
		page, err := renderTo(work, side+".html", "card.tmpl", p)
		if err != nil {
			return err
		}
		png, err := outPath(base + "-" + side + ".png")
		if err != nil {
			return err
		}
		w, h := trimW, trimH
		if c.Guides {
			w, h = pageW, pageH
		}
		if err := screenshot(chrome, work, page, png, w, h, previewScale); err != nil {
			return err
		}
		written = append(written, png)
	}

	fmt.Printf("QR code: %s (%d×%d modules of %.2f mm, %.1f mm symbol)\n", target, modules, modules, module, qrSymbolMM)
	for _, w := range written {
		fmt.Println("wrote", w)
	}
	if c.Proof {
		fmt.Println("PROOF copy: stamped and not for printing")
	}
	return nil
}

// qrSVG draws the QR code as vector paths: one subpath per horizontal run of
// dark modules (merging runs avoids hairline seams between abutting squares
// in some RIPs), inside a quiet zone of qrQuietModules. It returns the SVG and
// the symbol's width in modules.
func qrSVG(target string) (template.HTML, int, error) {
	q, err := qrcode.New(target, qrcode.Medium)
	if err != nil {
		return "", 0, fmt.Errorf("QR code: %w", err)
	}
	q.DisableBorder = true
	bm := q.Bitmap()
	n := len(bm)
	size := n + 2*qrQuietModules
	var d strings.Builder
	for y, row := range bm {
		for x := 0; x < len(row); {
			if !row[x] {
				x++
				continue
			}
			x0 := x
			for x < len(row) && row[x] {
				x++
			}
			fmt.Fprintf(&d, "M%d %dh%dv1h-%dz", x0+qrQuietModules, y+qrQuietModules, x-x0, x-x0)
		}
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="QR code: %s"><path fill="#1D2622" d="%s"/></svg>`,
		size, size, html.EscapeString(target), d.String())
	return template.HTML(svg), n, nil
}

var mediaBox = regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+([\d.]+)\s+([\d.]+)\s*\]`)

// checkPDF confirms the PDF has exactly two 100 × 65 mm pages
// (283.46 × 184.25 pt). The page dictionaries are plain text (see pdfbox.go),
// so a regex over the file is enough.
func checkPDF(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	boxes := mediaBox.FindAllSubmatch(b, -1)
	if len(boxes) != 2 {
		return fmt.Errorf("%s has %d pages, want 2 (front, back)", path, len(boxes))
	}
	const wantW, wantH = 100 * ptPerMM, 65 * ptPerMM
	for i, m := range boxes {
		w, _ := strconv.ParseFloat(string(m[1]), 64)
		h, _ := strconv.ParseFloat(string(m[2]), 64)
		if abs(w-wantW) > 0.01 || abs(h-wantH) > 0.01 {
			return fmt.Errorf("%s page %d is %.2f × %.2f pt, want %.2f × %.2f (100 × 65 mm)", path, i+1, w, h, wantW, wantH)
		}
	}
	return nil
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
