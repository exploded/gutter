// Command genassets renders the site's icon set and social images into
// static/img from the brand sources: the mark in static/img/logo-mark.svg and
// the TTFs in tools/brand/fonts. It's a dev-time tool — the PNGs are
// committed, and this only needs re-running if the mark, palette or wording
// changes. Run it from the repo root:
//
//	go run ./tools/genassets                  // static/img, plus cmd/server/logo.png for invoice PDFs
//	go run ./tools/genassets -pdf-logo=false  // static/img only
//	go run ./tools/genassets some/dir         // anywhere else (never touches cmd/server)
//
// It's pure Go: the mark is parsed from the SVG and rasterised with
// golang.org/x/image/vector, so the icons can't drift from the SVG, and text
// is set with golang.org/x/image/font/opentype plus the fonts' GPOS pair
// kerning (neither font has an old-style kern table, and display sizes show
// the difference).
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Palette — the brand spec, matching the :root tokens in static/css/styles.css.
var (
	ink        = rgb(0x12284A) // navy: text, the badge, dark fields
	inkDark    = rgb(0x0B1A33)
	tangerine  = rgb(0xFF7A21) // the spout; fills only on light
	tangerineT = rgb(0xB34700) // tangerine text on light
	sky        = rgb(0x4DB0FF) // water
	sun        = rgb(0xFFD449) // highlights, and text on dark
	cream      = rgb(0xFFF7EA)
)

const (
	markSVG  = "static/img/logo-mark.svg"
	fontDir  = "tools/brand/fonts"
	domain   = "upthespout.com.au"
	wordmark = "Up The Spout"
	subline  = "GUTTER CLEANING"
	pitch    = "Clean gutters, done properly."
	areaLine = "Melbourne's east and north-east  ·  Fixed prices  ·  Book online"
)

var services = []string{"Gutters", "Downpipes", "Clean-up", "Photo report"}

func main() {
	log.SetFlags(0)
	log.SetPrefix("genassets: ")
	pdfLogo := flag.Bool("pdf-logo", true, "also write cmd/server/logo.png (the invoice PDF's copy of the icon) when writing to static/img")
	flag.Parse()
	out := "static/img"
	if flag.NArg() > 0 {
		out = flag.Arg(0)
	}
	if _, err := os.Stat(fontDir); err != nil {
		log.Fatalf("%s not found: run this from the repo root", fontDir)
	}
	loadBrand()
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}
	must := func(err error) {
		if err != nil {
			log.Fatal(err)
		}
	}
	must(writePNG(filepath.Join(out, "og.png"), ogImage()))
	must(writePNG(filepath.Join(out, "square.png"), squareImage()))
	must(writePNG(filepath.Join(out, "cover.png"), coverImage()))
	for _, sz := range []int{32, 192, 512, 1024} {
		must(writePNG(filepath.Join(out, fmt.Sprintf("icon-%d.png", sz)), icon(sz)))
	}
	must(writePNG(filepath.Join(out, "apple-touch-icon.png"), appleTouchIcon(180)))
	must(writeICO(filepath.Join(out, "favicon.ico"), icon(16), icon(32), icon(48)))
	if *pdfLogo && filepath.Clean(out) == filepath.Clean("static/img") {
		must(writePNG("cmd/server/logo.png", icon(512)))
		fmt.Println("wrote cmd/server/logo.png")
	}
	fmt.Println("wrote assets to", out)
}

func rgb(v uint32) color.RGBA {
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
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

// writeICO wraps PNG-encoded images in an ICO container (PNG-in-ICO is
// supported by every browser since Vista-era Windows).
func writeICO(path string, imgs ...image.Image) error {
	var pngs [][]byte
	for _, im := range imgs {
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			return err
		}
		pngs = append(pngs, b.Bytes())
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&out, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&out, binary.LittleEndian, uint16(len(imgs)))
	offset := 6 + 16*len(imgs)
	for i, im := range imgs {
		w, h := im.Bounds().Dx(), im.Bounds().Dy()
		if w >= 256 {
			w = 0
		}
		if h >= 256 {
			h = 0
		}
		out.WriteByte(byte(w))
		out.WriteByte(byte(h))
		out.WriteByte(0)                                    // colour count
		out.WriteByte(0)                                    // reserved
		binary.Write(&out, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&out, binary.LittleEndian, uint16(32)) // bpp
		binary.Write(&out, binary.LittleEndian, uint32(len(pngs[i])))
		binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// ── brand sources ──

var (
	mark                              *svgMark
	lilita, sansReg, sansMed, sansB *typeface
)

func loadBrand() {
	var err error
	if mark, err = loadMark(markSVG); err != nil {
		log.Fatalf("%s: %v", markSVG, err)
	}
	load := func(name string) *typeface {
		tf, err := loadTypeface(filepath.Join(fontDir, name))
		if err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		return tf
	}
	lilita = load("LilitaOne-Regular.ttf")
	sansReg = load("Figtree-Regular.ttf")
	sansMed = load("Figtree-Medium.ttf")
	sansB = load("Figtree-ExtraBold.ttf")
}

// ── the mark: a small SVG reader and rasteriser ──
//
// It handles what logo-mark.svg uses — <rect> with rx, and <path> with
// M/L/H/V/C/A/Z (absolute or relative), filled or stroked with round caps and
// joins — and stops with an error on anything else rather than drawing the
// mark wrong.

type pt struct{ x, y float64 }

type svgShape struct {
	subpaths    [][]pt // flattened, in viewBox units
	closed      []bool
	fill        string // "" or "none" = no fill
	stroke      string
	strokeWidth float64
	roundCap    bool
	isBadge     bool // the <rect>
}

type svgMark struct {
	size   float64 // viewBox width (and height)
	shapes []svgShape
}

func loadMark(path string) (*svgMark, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	m := &svgMark{}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attr := map[string]string{}
		for _, a := range se.Attr {
			attr[a.Name.Local] = a.Value
		}
		switch se.Name.Local {
		case "svg":
			vb := strings.Fields(strings.ReplaceAll(attr["viewBox"], ",", " "))
			if len(vb) != 4 || vb[0] != "0" || vb[1] != "0" || vb[2] != vb[3] {
				return nil, fmt.Errorf("want a square viewBox at 0 0, got %q", attr["viewBox"])
			}
			m.size, _ = strconv.ParseFloat(vb[2], 64)
		case "rect":
			f := func(k string) float64 { v, _ := strconv.ParseFloat(attr[k], 64); return v }
			r := f("rx")
			if r == 0 {
				r = f("ry")
			}
			x, y := f("x"), f("y")
			sp := roundedRect(x, y, x+f("width"), y+f("height"), r)
			sh, err := shapeStyle(attr)
			if err != nil {
				return nil, err
			}
			sh.subpaths, sh.closed, sh.isBadge = [][]pt{sp}, []bool{true}, true
			m.shapes = append(m.shapes, sh)
		case "path":
			sh, err := shapeStyle(attr)
			if err != nil {
				return nil, err
			}
			if sh.subpaths, sh.closed, err = parsePath(attr["d"]); err != nil {
				return nil, err
			}
			m.shapes = append(m.shapes, sh)
		case "g", "title", "desc":
		default:
			return nil, fmt.Errorf("unsupported element <%s>", se.Name.Local)
		}
	}
	if m.size == 0 || len(m.shapes) == 0 {
		return nil, errors.New("no viewBox or no shapes")
	}
	return m, nil
}

func shapeStyle(attr map[string]string) (svgShape, error) {
	sh := svgShape{fill: attr["fill"], stroke: attr["stroke"]}
	if _, ok := attr["fill"]; !ok {
		sh.fill = "#000000"
	}
	if sh.stroke != "" && sh.stroke != "none" {
		sh.strokeWidth = 1
		if w, ok := attr["stroke-width"]; ok {
			sh.strokeWidth, _ = strconv.ParseFloat(w, 64)
		}
		switch attr["stroke-linecap"] {
		case "round":
			sh.roundCap = true
		case "", "butt":
		default:
			return sh, fmt.Errorf("unsupported stroke-linecap %q", attr["stroke-linecap"])
		}
		// Joins are always drawn round. Unset is fine too: in this mark that
		// only happens on paths without corners (one segment, or a smooth curve).
		if j := attr["stroke-linejoin"]; j != "" && j != "round" {
			return sh, fmt.Errorf("unsupported stroke-linejoin %q", j)
		}
	}
	return sh, nil
}

// parsePath flattens SVG path data into polylines.
func parsePath(d string) ([][]pt, []bool, error) {
	toks := tokenizePath(d)
	var subs [][]pt
	var closed []bool
	var cur []pt
	var p, start pt
	cmd := byte(0)
	i := 0
	num := func() (float64, error) {
		if i >= len(toks) {
			return 0, errors.New("path data ends early")
		}
		v, err := strconv.ParseFloat(toks[i], 64)
		i++
		return v, err
	}
	flush := func(isClosed bool) {
		if len(cur) > 1 {
			subs = append(subs, cur)
			closed = append(closed, isClosed)
		}
		cur = nil
	}
	for i < len(toks) {
		if c := toks[i][0]; (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			cmd = c
			i++
		} else if cmd == 0 {
			return nil, nil, fmt.Errorf("path data starts with %q", toks[i])
		}
		rel := cmd >= 'a'
		off := func(q pt) pt {
			if rel {
				return pt{q.x + p.x, q.y + p.y}
			}
			return q
		}
		read2 := func() (pt, error) {
			x, err := num()
			if err != nil {
				return pt{}, err
			}
			y, err := num()
			return pt{x, y}, err
		}
		switch cmd | 0x20 { // lower-case
		case 'm':
			q, err := read2()
			if err != nil {
				return nil, nil, err
			}
			flush(false)
			p = off(q)
			start = p
			cur = []pt{p}
			cmd = 'L' // further coordinate pairs are lines
			if rel {
				cmd = 'l'
			}
		case 'l':
			q, err := read2()
			if err != nil {
				return nil, nil, err
			}
			p = off(q)
			cur = append(cur, p)
		case 'h':
			x, err := num()
			if err != nil {
				return nil, nil, err
			}
			if rel {
				x += p.x
			}
			p = pt{x, p.y}
			cur = append(cur, p)
		case 'v':
			y, err := num()
			if err != nil {
				return nil, nil, err
			}
			if rel {
				y += p.y
			}
			p = pt{p.x, y}
			cur = append(cur, p)
		case 'c':
			var c [3]pt
			for k := range c {
				q, err := read2()
				if err != nil {
					return nil, nil, err
				}
				c[k] = off(q)
			}
			cur = append(cur, cubic(p, c[0], c[1], c[2])[1:]...)
			p = c[2]
		case 'a':
			var v [5]float64
			for k := range v {
				var err error
				if v[k], err = num(); err != nil {
					return nil, nil, err
				}
			}
			q, err := read2()
			if err != nil {
				return nil, nil, err
			}
			q = off(q)
			cur = append(cur, arc(p, q, v[0], v[1], v[2], v[3] != 0, v[4] != 0)[1:]...)
			p = q
		case 'z':
			if len(cur) > 0 && cur[len(cur)-1] != start {
				cur = append(cur, start)
			}
			flush(true)
			p = start
			cur = []pt{p}
		default:
			return nil, nil, fmt.Errorf("unsupported path command %q", string(cmd))
		}
	}
	flush(false)
	return subs, closed, nil
}

func tokenizePath(d string) []string {
	var toks []string
	var b strings.Builder
	emit := func() {
		if b.Len() > 0 {
			toks = append(toks, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(d); i++ {
		c := d[i]
		switch {
		case c == ' ' || c == ',' || c == '\n' || c == '\t' || c == '\r':
			emit()
		case (c >= 'A' && c <= 'Z' && c != 'E') || (c >= 'a' && c <= 'z' && c != 'e'):
			emit()
			toks = append(toks, string(c))
		case c == '-' && b.Len() > 0 && !strings.HasSuffix(b.String(), "e") && !strings.HasSuffix(b.String(), "E"):
			emit()
			b.WriteByte(c)
		case c == '.' && strings.Contains(b.String(), "."):
			emit()
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	emit()
	return toks
}

const curveSteps = 48

func cubic(p0, p1, p2, p3 pt) []pt {
	out := make([]pt, 0, curveSteps+1)
	for k := 0; k <= curveSteps; k++ {
		t := float64(k) / curveSteps
		u := 1 - t
		a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		out = append(out, pt{a*p0.x + b*p1.x + c*p2.x + d*p3.x, a*p0.y + b*p1.y + c*p2.y + d*p3.y})
	}
	return out
}

// arc flattens an SVG elliptical arc (endpoint form; SVG 1.1 appendix F.6.5).
func arc(p0, p1 pt, rx, ry, rotDeg float64, large, sweep bool) []pt {
	if rx == 0 || ry == 0 || p0 == p1 {
		return []pt{p0, p1}
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	phi := rotDeg * math.Pi / 180
	cs, sn := math.Cos(phi), math.Sin(phi)
	dx, dy := (p0.x-p1.x)/2, (p0.y-p1.y)/2
	x1 := cs*dx + sn*dy
	y1 := -sn*dx + cs*dy
	if l := x1*x1/(rx*rx) + y1*y1/(ry*ry); l > 1 {
		rx, ry = rx*math.Sqrt(l), ry*math.Sqrt(l)
	}
	num := rx*rx*ry*ry - rx*rx*y1*y1 - ry*ry*x1*x1
	den := rx*rx*y1*y1 + ry*ry*x1*x1
	co := math.Sqrt(math.Max(0, num/den))
	if large == sweep {
		co = -co
	}
	cx1, cy1 := co*rx*y1/ry, -co*ry*x1/rx
	cx := cs*cx1 - sn*cy1 + (p0.x+p1.x)/2
	cy := sn*cx1 + cs*cy1 + (p0.y+p1.y)/2
	ang := func(ux, uy, vx, vy float64) float64 { return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy) }
	t0 := ang(1, 0, (x1-cx1)/rx, (y1-cy1)/ry)
	dt := ang((x1-cx1)/rx, (y1-cy1)/ry, (-x1-cx1)/rx, (-y1-cy1)/ry)
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	out := []pt{p0}
	for k := 1; k <= curveSteps; k++ {
		t := t0 + dt*float64(k)/curveSteps
		ex, ey := rx*math.Cos(t), ry*math.Sin(t)
		out = append(out, pt{cs*ex - sn*ey + cx, sn*ex + cs*ey + cy})
	}
	out[len(out)-1] = p1
	return out
}

func roundedRect(x0, y0, x1, y1, r float64) []pt {
	r = math.Min(r, math.Min(x1-x0, y1-y0)/2)
	var out []pt
	corner := func(cx, cy, a0 float64) {
		for k := 0; k <= curveSteps/2; k++ {
			a := a0 + math.Pi/2*float64(k)/float64(curveSteps/2)
			out = append(out, pt{cx + r*math.Cos(a), cy + r*math.Sin(a)})
		}
	}
	corner(x1-r, y0+r, -math.Pi/2)
	corner(x1-r, y1-r, 0)
	corner(x0+r, y1-r, math.Pi/2)
	corner(x0+r, y0+r, math.Pi)
	return append(out, out[0])
}

// polygonArea is the signed area; the sign gives the winding direction.
func polygonArea(p []pt) float64 {
	a := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i].x*p[j].y - p[j].x*p[i].y
	}
	return a / 2
}

// wound returns p with a negative signed area. vector.Rasterizer adds up
// signed coverage and takes its absolute value, so pieces of one stroke must
// all wind the same way or their overlaps cancel into holes.
func wound(p []pt) []pt {
	if polygonArea(p) > 0 {
		q := make([]pt, len(p))
		for i := range p {
			q[i] = p[len(p)-1-i]
		}
		return q
	}
	return p
}

func disc(c pt, r float64) []pt {
	out := make([]pt, 0, 64)
	for k := 0; k < 64; k++ {
		a := 2 * math.Pi * float64(k) / 64
		out = append(out, pt{c.x + r*math.Cos(a), c.y + r*math.Sin(a)})
	}
	return wound(out)
}

// strokePolyline outlines a polyline as quads per segment plus a disc at every
// vertex: round joins everywhere, round caps when asked.
func strokePolyline(p []pt, w float64, roundCap, isClosed bool) [][]pt {
	h := w / 2
	var out [][]pt
	for i := 0; i+1 < len(p); i++ {
		a, b := p[i], p[i+1]
		dx, dy := b.x-a.x, b.y-a.y
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		nx, ny := -dy/l*h, dx/l*h
		out = append(out, wound([]pt{{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny}, {b.x - nx, b.y - ny}, {a.x - nx, a.y - ny}}))
	}
	for i, v := range p {
		end := !isClosed && (i == 0 || i == len(p)-1)
		if end && !roundCap {
			continue
		}
		out = append(out, disc(v, h))
	}
	return out
}

// markStyle recolours the mark for its surface.
type markStyle struct {
	recolor map[string]color.Color // SVG colour (upper-case hex) → colour to use
	edge    color.Color            // hairline around the badge; nil for none
	small   bool                   // drop details under a pixel wide
}

var (
	standardMark = markStyle{}
	// onInkMark is the treatment for navy surfaces (the card front, the
	// cover): the badge would vanish into the field, so it goes a shade darker
	// with a hairline cream edge, and the leaf vein follows the badge colour.
	onInkMark = markStyle{
		recolor: map[string]color.Color{"#12284A": inkDark},
		edge:    color.NRGBA{0xFF, 0xF7, 0xEA, 0x8C},
	}
)

func (st markStyle) colour(svg string) color.Color {
	svg = strings.ToUpper(svg)
	if c, ok := st.recolor[svg]; ok {
		return c
	}
	if strings.HasPrefix(svg, "#") && len(svg) == 7 {
		v, err := strconv.ParseUint(svg[1:], 16, 32)
		if err == nil {
			return rgb(uint32(v))
		}
	}
	log.Fatalf("unsupported colour %q in %s", svg, markSVG)
	return nil
}

// drawMark paints the mark with its viewBox scaled to size px at (x, y).
func drawMark(dst *image.RGBA, x, y, size float64, st markStyle) {
	s := size / mark.size
	tx := func(polys [][]pt) [][]pt {
		out := make([][]pt, len(polys))
		for i, p := range polys {
			q := make([]pt, len(p))
			for j, v := range p {
				q[j] = pt{x + v.x*s, y + v.y*s}
			}
			out[i] = q
		}
		return out
	}
	for _, sh := range mark.shapes {
		if sh.fill != "" && sh.fill != "none" {
			fillPolys(dst, tx(sh.subpaths), st.colour(sh.fill))
		}
		if sh.stroke != "" && sh.stroke != "none" {
			if st.small && sh.strokeWidth*s < 1 {
				continue // a sub-pixel stroke only muddies a favicon
			}
			var polys [][]pt
			for i, sp := range sh.subpaths {
				polys = append(polys, strokePolyline(sp, sh.strokeWidth, sh.roundCap, sh.closed[i])...)
			}
			fillPolys(dst, tx(polys), st.colour(sh.stroke))
		}
		if sh.isBadge && st.edge != nil {
			var polys [][]pt
			for _, sp := range sh.subpaths {
				polys = append(polys, strokePolyline(sp, 0.9, true, true)...)
			}
			fillPolys(dst, tx(polys), st.edge)
		}
	}
}

func fillPolys(dst *image.RGBA, polys [][]pt, col color.Color) {
	b := dst.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	for _, p := range polys {
		if len(p) < 3 {
			continue
		}
		z.MoveTo(float32(p[0].x-float64(b.Min.X)), float32(p[0].y-float64(b.Min.Y)))
		for _, v := range p[1:] {
			z.LineTo(float32(v.x-float64(b.Min.X)), float32(v.y-float64(b.Min.Y)))
		}
		z.ClosePath()
	}
	z.Draw(dst, b, image.NewUniform(col), image.Point{})
}

// ── type: faces with GPOS pair kerning and tracking ──

type typeface struct {
	f     *opentype.Font
	kern  *kernTable
	upm   float64
	buf   sfnt.Buffer
	faces map[float64]font.Face
}

func loadTypeface(path string) (*typeface, error) {
	ttf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := opentype.Parse(ttf)
	if err != nil {
		return nil, err
	}
	k, err := parseKern(ttf)
	if err != nil {
		return nil, fmt.Errorf("GPOS kerning: %w", err)
	}
	return &typeface{f: f, kern: k, upm: float64(f.UnitsPerEm()), faces: map[float64]font.Face{}}, nil
}

func (tf *typeface) face(px float64) font.Face {
	if fc, ok := tf.faces[px]; ok {
		return fc
	}
	fc, err := opentype.NewFace(tf.f, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		log.Fatal(err)
	}
	tf.faces[px] = fc
	return fc
}

func (tf *typeface) glyph(r rune) sfnt.GlyphIndex {
	g, err := tf.f.GlyphIndex(&tf.buf, r)
	if err != nil {
		log.Fatal(err)
	}
	return g
}

// style is a typeface at a pixel size, with tracking in em (CSS letter-spacing).
type style struct {
	tf    *typeface
	px    float64
	track float64
}

// layout returns each rune's x offset from the start and the total advance,
// less the tracking after the last letter (so centring is optical).
func (s style) layout(str string) ([]float64, float64) {
	fc := s.tf.face(s.px)
	var xs []float64
	x := 0.0
	var prev sfnt.GlyphIndex
	for i, r := range []rune(str) {
		g := s.tf.glyph(r)
		if i > 0 {
			x += float64(s.tf.kern.pair(prev, g)) * s.px / s.tf.upm
			x += s.track * s.px
		}
		xs = append(xs, x)
		adv, _ := fc.GlyphAdvance(r)
		x += float64(adv) / 64
		prev = g
	}
	return xs, x
}

func (s style) width(str string) float64 { _, w := s.layout(str); return w }

// capHeight is the height of "H" above the baseline, in px.
func (s style) capHeight() float64 {
	b, _, err := s.tf.f.GlyphBounds(&s.tf.buf, s.tf.glyph('H'), fixed.Int26_6(s.px*64), font.HintingNone)
	if err != nil {
		log.Fatal(err)
	}
	return -float64(b.Min.Y) / 64
}

// draw sets str with its baseline at y, starting at x; returns the end x.
func (s style) draw(dst draw.Image, str string, x, y float64, col color.Color) float64 {
	fc := s.tf.face(s.px)
	xs, w := s.layout(str)
	src := image.NewUniform(col)
	for i, r := range []rune(str) {
		dot := fixed.Point26_6{X: fixed.Int26_6(math.Round((x + xs[i]) * 64)), Y: fixed.Int26_6(math.Round(y * 64))}
		dr, mask, mp, _, ok := fc.Glyph(dot, r)
		if !ok {
			log.Fatalf("no glyph for %q", r)
		}
		draw.DrawMask(dst, dr, src, image.Point{}, mask, mp, draw.Over)
	}
	return x + w
}

// drawC centres str on cx.
func (s style) drawC(dst draw.Image, str string, cx, y float64, col color.Color) {
	s.draw(dst, str, cx-s.width(str)/2, y, col)
}

// kernTable reads pair adjustments (lookup type 2, via type 9 extensions too)
// from the lookups of a font's GPOS "kern" feature — enough for Latin text.
type kernTable struct {
	gpos    []byte
	lookups [][]int // subtable offsets into gpos, per lookup, in lookup order
}

func parseKern(ttf []byte) (*kernTable, error) {
	k := &kernTable{}
	if len(ttf) < 12 {
		return nil, errors.New("short font file")
	}
	u16 := func(b []byte, o int) int {
		if o+2 > len(b) {
			return 0
		}
		return int(binary.BigEndian.Uint16(b[o:]))
	}
	u32 := func(b []byte, o int) int {
		if o+4 > len(b) {
			return 0
		}
		return int(binary.BigEndian.Uint32(b[o:]))
	}
	for i := 0; i < u16(ttf, 4); i++ {
		rec := 12 + 16*i
		if string(ttf[rec:rec+4]) == "GPOS" {
			off, n := u32(ttf, rec+8), u32(ttf, rec+12)
			if off+n > len(ttf) {
				return nil, errors.New("GPOS table runs past the end of the file")
			}
			k.gpos = ttf[off : off+n]
		}
	}
	if k.gpos == nil {
		return k, nil // no GPOS: no kerning
	}
	g := k.gpos
	featList, lookList := u16(g, 6), u16(g, 8)
	want := map[int]bool{}
	for i := 0; i < u16(g, featList); i++ {
		rec := featList + 2 + 6*i
		if string(g[rec:rec+4]) != "kern" {
			continue
		}
		feat := featList + u16(g, rec+4)
		for j := 0; j < u16(g, feat+2); j++ {
			want[u16(g, feat+4+2*j)] = true
		}
	}
	for li := 0; li < u16(g, lookList); li++ {
		if !want[li] {
			continue
		}
		look := lookList + u16(g, lookList+2+2*li)
		typ := u16(g, look)
		var subs []int
		for s := 0; s < u16(g, look+4); s++ {
			sub := look + u16(g, look+6+2*s)
			t := typ
			if t == 9 { // extension: the real subtable is further on
				t = u16(g, sub+2)
				sub += u32(g, sub+4)
			}
			if t == 2 {
				subs = append(subs, sub)
			}
		}
		if len(subs) > 0 {
			k.lookups = append(k.lookups, subs)
		}
	}
	return k, nil
}

func (k *kernTable) u16(o int) int {
	if o+2 > len(k.gpos) {
		return 0
	}
	return int(binary.BigEndian.Uint16(k.gpos[o:]))
}

// pair returns the x-advance adjustment for g1 followed by g2, in font units.
func (k *kernTable) pair(g1, g2 sfnt.GlyphIndex) int {
	total := 0
	for _, subs := range k.lookups {
		for _, sub := range subs {
			if v, ok := k.pairPos(sub, int(g1), int(g2)); ok {
				total += v
				break // first matching subtable wins within a lookup
			}
		}
	}
	return total
}

func (k *kernTable) pairPos(sub, g1, g2 int) (int, bool) {
	cov, ok := k.coverage(sub+k.u16(sub+2), g1)
	if !ok {
		return 0, false
	}
	vf1, vf2 := k.u16(sub+4), k.u16(sub+6)
	size1, size2 := 2*bits.OnesCount16(uint16(vf1)), 2*bits.OnesCount16(uint16(vf2))
	xAdv := func(rec int) int {
		if vf1&0x4 == 0 {
			return 0
		}
		return int(int16(k.u16(rec + 2*bits.OnesCount16(uint16(vf1&0x3)))))
	}
	switch k.u16(sub) {
	case 1:
		set := sub + k.u16(sub+10+2*cov)
		recSize := 2 + size1 + size2
		for i := 0; i < k.u16(set); i++ {
			rec := set + 2 + i*recSize
			if k.u16(rec) == g2 {
				return xAdv(rec + 2), true
			}
		}
		return 0, false
	case 2:
		c1 := k.class(sub+k.u16(sub+8), g1)
		c2 := k.class(sub+k.u16(sub+10), g2)
		n1, n2 := k.u16(sub+12), k.u16(sub+14)
		if c1 >= n1 || c2 >= n2 {
			return 0, true
		}
		return xAdv(sub + 16 + (c1*n2+c2)*(size1+size2)), true
	}
	return 0, false
}

func (k *kernTable) coverage(off, g int) (int, bool) {
	switch k.u16(off) {
	case 1:
		for i := 0; i < k.u16(off+2); i++ {
			if k.u16(off+4+2*i) == g {
				return i, true
			}
		}
	case 2:
		for i := 0; i < k.u16(off+2); i++ {
			r := off + 4 + 6*i
			if start, end := k.u16(r), k.u16(r+2); g >= start && g <= end {
				return k.u16(r+4) + g - start, true
			}
		}
	}
	return 0, false
}

func (k *kernTable) class(off, g int) int {
	switch k.u16(off) {
	case 1:
		start, n := k.u16(off+2), k.u16(off+4)
		if g >= start && g < start+n {
			return k.u16(off + 6 + 2*(g-start))
		}
	case 2:
		for i := 0; i < k.u16(off+2); i++ {
			r := off + 4 + 6*i
			if g >= k.u16(r) && g <= k.u16(r+2) {
				return k.u16(r + 4)
			}
		}
	}
	return 0
}

// ── drawing helpers ──

func fill(dst draw.Image, r image.Rectangle, col color.Color) {
	draw.Draw(dst, r, image.NewUniform(col), image.Point{}, draw.Src)
}

// roundRect fills an anti-aliased rounded rectangle (float coordinates).
func roundRect(dst *image.RGBA, x0, y0, x1, y1, r float64, col color.Color) {
	fillPolys(dst, [][]pt{roundedRect(x0, y0, x1, y1, r)}, col)
}

// outlineRect strokes a rounded rectangle's outline.
func outlineRect(dst *image.RGBA, x0, y0, x1, y1, r, w float64, col color.Color) {
	fillPolys(dst, strokePolyline(roundedRect(x0+w/2, y0+w/2, x1-w/2, y1-w/2, r-w/2), w, true, true), col)
}

// gradient paints a diagonal two-stop fade across the whole image.
func gradient(dst *image.RGBA, from, to color.RGBA) {
	b := dst.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	lerp := func(a, c uint8, t float64) uint8 { return uint8(float64(a)*(1-t) + float64(c)*t + 0.5) }
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			t := math.Max(0, math.Min(1, float64(x)/w*0.35+float64(y)/h*0.65))
			dst.SetRGBA(x, y, color.RGBA{lerp(from.R, to.R, t), lerp(from.G, to.G, t), lerp(from.B, to.B, t), 0xff})
		}
	}
}

// softGlow paints a faint radial blob over an opaque image.
func softGlow(dst *image.RGBA, cx, cy, radius float64, col color.RGBA, maxAlpha float64) {
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / radius
			if d >= 1 {
				continue
			}
			a := (1 - d) * (1 - d) * maxAlpha
			c := dst.RGBAAt(x, y)
			mix := func(bg, fg uint8) uint8 { return uint8(float64(bg)*(1-a) + float64(fg)*a + 0.5) }
			dst.SetRGBA(x, y, color.RGBA{mix(c.R, col.R), mix(c.G, col.G), mix(c.B, col.B), 0xff})
		}
	}
}

// mustFit fails the build rather than shipping an image a crop would clip —
// the wording here changes more often than anyone re-checks the crop.
func mustFit(w, safe float64, what string) {
	if w > safe+0.5 {
		log.Fatalf("%s is %.0fpx, over the %.0fpx crop budget", what, w, safe)
	}
}

// ── the lockup ──
//
// Mark on the left; "Up The Spout" in Lilita One (tracking 0.01 em) over
// "GUTTER CLEANING" in Figtree ExtraBold at 30% of its size (tracking 0.24
// em). em is the wordmark size in px. The geometry matches
// tools/bizcard/brand.tmpl: mark 1.5 em square, 0.34 em gap, and the subline
// cap top 0.25 em below the wordmark baseline, the word block centred on the
// mark.

type lockup struct {
	em         float64
	word, sub  color.Color
	markStyle  markStyle
	words, gut style
}

func newLockup(em float64, word, sub color.Color, ms markStyle) lockup {
	return lockup{em: em, word: word, sub: sub, markStyle: ms,
		words: style{lilita, em, 0.01},
		gut:   style{sansB, em * 0.30, 0.24}}
}

func (l lockup) markSize() float64 { return 1.5 * l.em }
func (l lockup) height() float64   { return l.markSize() }
func (l lockup) width() float64 {
	return l.markSize() + 0.34*l.em + math.Max(l.words.width(wordmark), l.gut.width(subline))
}

// draw places the lockup's top-left corner at (x, y).
func (l lockup) draw(dst *image.RGBA, x, y float64) {
	m := l.markSize()
	drawMark(dst, x, y, m, l.markStyle)
	capW, capG := l.words.capHeight(), l.gut.capHeight()
	gap := 0.25 * l.em
	top := y + (m-(capW+gap+capG))/2
	tx := x + m + 0.34*l.em
	l.words.draw(dst, wordmark, tx, top+capW, l.word)
	l.gut.draw(dst, subline, tx, top+capW+gap+capG, l.sub)
}

// ── the images ──

// icon is the mark on a transparent background, as the SVG draws it.
func icon(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	st := standardMark
	st.small = size < 64
	drawMark(img, 0, 0, float64(size), st)
	return img
}

// appleTouchIcon has no transparency: iOS fills transparent pixels with black
// and applies its own rounded mask, so the badge navy runs into the corners.
func appleTouchIcon(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	fill(img, img.Bounds(), ink)
	drawMark(img, 0, 0, float64(size), standardMark)
	return img
}

// ogImage is the link-preview card (1200×630), shown whole: cream field, a
// big sun coming up off the top-right corner, the lockup and pitch on the
// left, and a navy footer band carrying the domain and the area.
func ogImage() *image.RGBA {
	const W, H = 1200, 630
	const margin = 84
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	fill(img, img.Bounds(), cream)

	l := newLockup(86, ink, tangerineT, standardMark)
	mustFit(l.width(), W-2*margin, "og lockup")
	p := style{sansMed, 34, 0}
	mustFit(p.width(pitch), 760, "og pitch (it must clear the sun)")

	// The sun: an ink-ringed disc, mostly off the canvas, behind everything.
	fillPolys(img, [][]pt{disc(pt{1090, 70}, 236)}, ink)
	fillPolys(img, [][]pt{disc(pt{1090, 70}, 230)}, sun)

	// The text block (lockup top to services baseline) is centred in the
	// cream above the band.
	band := 128
	const pitchOff, svcOff = 78, 130 // baselines below the lockup
	blockH := l.height() + svcOff
	top := math.Round((float64(H-band) - blockH) / 2)

	fill(img, image.Rect(0, H-band, W, H), ink)
	fill(img, image.Rect(0, H-band, W, H-band+8), tangerine)

	l.draw(img, margin, top)
	p.draw(img, pitch, margin, top+l.height()+pitchOff, ink)
	svc := style{sansB, 24, 0.01}
	line := strings.Join(services, "  ·  ")
	mustFit(svc.width(line), W-2*margin, "og services")
	svc.draw(img, line, margin, top+l.height()+svcOff, tangerineT)

	d := style{sansB, 30, 0.01}
	d.draw(img, domain, margin, float64(H-band/2)+14, cream)
	area := style{sansMed, 20, 0.01}
	mustFit(d.width(domain)+48+area.width(areaLine), W-2*margin, "og footer")
	area.draw(img, areaLine, W-margin-area.width(areaLine), float64(H-band/2)+11, sun)
	return img
}

// brandPlate draws the centred brand stack — lockup, tangerine rule, pitch,
// service chips and domain — full-bleed on a navy field, in the same
// treatment as the business card front. Cover and square differ only in
// canvas shape and scale, so they share it.
//
// safeW and safeH are the narrowest and shortest crops the slot can take, less
// a margin: everything is held inside them so no crop can clip a word. Google
// Business Profile re-crops per surface, so nothing is drawn near an edge
// either — a crop that sliced through a chip would look broken.
func brandPlate(img *image.RGBA, scale float64, safeW, safeH float64) {
	W, H := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	cx := W / 2
	s := func(v float64) float64 { return v * scale }

	gradient(img, ink, inkDark)
	softGlow(img, W*0.88, H*0.08, W*0.5, sky, 0.14)
	softGlow(img, W*0.08, H*0.98, W*0.42, sky, 0.12)

	// Offsets down from the top of the stack, at scale 1. The block is
	// measured and centred as a whole, so it sits right on any canvas shape.
	const (
		lockEm  = 80
		ruleY   = 160 // top of the rule
		pitchY  = 230 // baselines from here on
		pitch2Y = 270
		chipY   = 306 // top of the chip row
		chipH   = 42
		domainY = 398
		stackH  = 404
	)
	mustFit(s(stackH), safeH, "stack height")
	top := (H - s(stackH)) / 2
	at := func(off float64) float64 { return top + s(off) }

	l := newLockup(s(lockEm), cream, sun, onInkMark)
	mustFit(l.width(), safeW, "lockup")
	l.draw(img, cx-l.width()/2, top)

	roundRect(img, cx-s(26), at(ruleY), cx+s(26), at(ruleY)+s(6), s(3), tangerine)

	// The pitch, split at its comma: one line is wider than a square crop allows.
	p := style{sansMed, s(30), 0}
	line1, line2, _ := strings.Cut(pitch, ", ")
	line1 += ","
	mustFit(math.Max(p.width(line1), p.width(line2)), safeW, "pitch")
	p.drawC(img, line1, cx, at(pitchY), cream)
	p.drawC(img, line2, cx, at(pitch2Y), cream)

	// Service chips, centred as a row: outlined rather than filled, so they
	// read as trim on the field instead of competing with the wordmark.
	chip := style{sansMed, s(20), 0.01}
	gap, padX := s(12), s(18)
	total := gap * float64(len(services)-1)
	for _, c := range services {
		total += chip.width(c) + padX*2
	}
	mustFit(total, safeW, "chip row")
	x := cx - total/2
	for _, c := range services {
		bw := chip.width(c) + padX*2
		outlineRect(img, x, at(chipY), x+bw, at(chipY)+s(chipH), s(chipH)/2, s(1.8), color.NRGBA{0xFF, 0xF7, 0xEA, 0x80})
		chip.draw(img, c, x+padX, at(chipY)+s(chipH)/2+chip.capHeight()/2, cream)
		x += bw + gap
	}

	d := style{sansB, s(24), 0.02}
	mustFit(d.width(domain), safeW, "domain")
	d.drawC(img, domain, cx, at(domainY), sun)
}

// coverImage is the 16:9 cover, sized for Google Business Profile. Unlike
// ogImage — a link-preview card that is only ever shown whole — this is
// full-bleed, because the worst case here is a centred square crop, which of a
// 1200×675 image keeps only the middle 675px.
func coverImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 1200, 675))
	brandPlate(img, 1, 620, 675) // a square crop keeps the full height, so only width binds
	return img
}

// squareImage is the 1:1 variant, for the Google Business Profile photos slot.
// Photos are shown square or cropped to 4:3 landscape — which of a square keeps
// the middle 900px of height and none of the width — so the height is the tight
// dimension here, the reverse of the cover. It runs the stack larger, because
// photos are usually seen small, as a thumbnail beside the profile.
func squareImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 1200, 1200))
	brandPlate(img, 1.55, 1000, 880)
	return img
}
