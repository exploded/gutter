package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Chrome snaps custom @page sizes to its own grid: "100mm 65mm" comes out as
// 282.96 × 184.08 pt, not 283.46 × 184.25, which would shave the bleed. So the
// pages are printed a little oversize (the content laid out from the top-left
// corner as usual) and exactPages then rewrites the PDF: each page's MediaBox
// becomes exactly w × h pt, the content is shifted down so the top-left
// corner stays put, and, for print, TrimBox and BleedBox are set so the
// printer's software knows where to cut.
//
// It understands the PDFs Chrome (Skia) writes: a classic xref table and
// plain-text page dictionaries. Anything else is an error, not a guess.

const ptPerMM = 72 / 25.4

var (
	reStartXref = regexp.MustCompile(`startxref\s+(\d+)\s+%%EOF\s*$`)
	reXrefSub   = regexp.MustCompile(`^(\d+) (\d+)\s*$`)
	reTrailer   = regexp.MustCompile(`(?s)trailer\s*<<(.*)>>\s*startxref`)
	reSize      = regexp.MustCompile(`/Size\s+\d+`)
	rePageType  = regexp.MustCompile(`/Type\s*/Page[^s]`)
	reMediaBox  = regexp.MustCompile(`/MediaBox\s*\[\s*([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s*\]`)
	reContents  = regexp.MustCompile(`/Contents\s*(\[[^\]]*\]|\d+\s+\d+\s+R)`)
)

// exactPages resizes every page of the PDF at path to w × h pt, keeping the
// top-left corner fixed. With trim > 0 it also sets TrimBox (inset by trim pt
// on every side) and BleedBox (the whole page).
func exactPages(path string, w, h, trim float64) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := resizePDF(data, w, h, trim)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return os.WriteFile(path, out, 0o644)
}

func resizePDF(data []byte, w, h, trim float64) ([]byte, error) {
	m := reStartXref.FindSubmatch(data)
	if m == nil {
		return nil, errors.New("no startxref; not a PDF layout this tool understands")
	}
	xrefAt, _ := strconv.Atoi(string(m[1]))
	if xrefAt <= 0 || xrefAt >= len(data) || !bytes.HasPrefix(data[xrefAt:], []byte("xref")) {
		return nil, errors.New("no classic xref table (an xref stream?); can't fix the page size")
	}

	// Read the xref table: object number → byte offset, for objects in use.
	offsets := map[int]int{}
	lines := strings.Split(strings.ReplaceAll(string(data[xrefAt:]), "\r", ""), "\n")
	size := 0
	for i := 1; i < len(lines); {
		sub := reXrefSub.FindStringSubmatch(lines[i])
		if sub == nil {
			break
		}
		first, _ := strconv.Atoi(sub[1])
		count, _ := strconv.Atoi(sub[2])
		i++
		for k := 0; k < count; k++ {
			if i+k >= len(lines) {
				return nil, errors.New("truncated xref table")
			}
			f := strings.Fields(lines[i+k])
			if len(f) == 3 && f[2] == "n" {
				off, err := strconv.Atoi(f[0])
				if err != nil {
					return nil, fmt.Errorf("bad xref entry %q", lines[i+k])
				}
				offsets[first+k] = off
			}
		}
		i += count
		size = max(size, first+count)
	}
	tm := reTrailer.FindSubmatch(data[xrefAt:])
	if tm == nil || len(offsets) == 0 {
		return nil, errors.New("couldn't read the xref table or trailer")
	}
	if bytes.Contains(tm[1], []byte("/Prev")) {
		return nil, errors.New("PDF has incremental updates; expected a fresh file from Chrome")
	}

	// Objects are contiguous, so each one runs to the start of the next.
	nums := make([]int, 0, len(offsets))
	for n := range offsets {
		nums = append(nums, n)
	}
	sort.Slice(nums, func(a, b int) bool { return offsets[nums[a]] < offsets[nums[b]] })
	body := map[int][]byte{}
	for i, n := range nums {
		end := xrefAt
		if i+1 < len(nums) {
			end = offsets[nums[i+1]]
		}
		body[n] = bytes.TrimRight(data[offsets[n]:end], " \r\n")
	}

	var extra [][]byte // new objects, numbered from size up
	pages := 0
	for _, n := range nums {
		obj := body[n]
		head, _, hasStream := bytes.Cut(obj, []byte("stream"))
		if hasStream || !rePageType.Match(head) {
			continue
		}
		pages++
		if bytes.Contains(obj, []byte("/Annots")) {
			return nil, errors.New("page has annotations (links?), which this resize doesn't move")
		}
		mb := reMediaBox.FindSubmatch(obj)
		if mb == nil {
			return nil, fmt.Errorf("page object %d has no MediaBox", n)
		}
		var box [4]float64
		for i := range box {
			box[i], _ = strconv.ParseFloat(string(mb[i+1]), 64)
		}
		if box[0] != 0 || box[1] != 0 {
			return nil, fmt.Errorf("page object %d MediaBox doesn't start at 0 0", n)
		}
		if box[2] < w-0.01 || box[3] < h-0.01 {
			return nil, fmt.Errorf("Chrome made a %.2f × %.2f pt page, smaller than the %.2f × %.2f pt wanted; print it larger", box[2], box[3], w, h)
		}

		// A content stream that runs first and shifts everything down, so
		// what was at the top of the tall page is at the top of the new one.
		shift := fmt.Sprintf("1 0 0 1 0 %s cm\n", num(h-box[3]))
		extraNum := size + len(extra)
		extra = append(extra, fmt.Appendf(nil, "%d 0 obj\n<</Length %d>> stream\n%sendstream\nendobj", extraNum, len(shift), shift))

		cm := reContents.FindSubmatch(obj)
		if cm == nil {
			return nil, fmt.Errorf("page object %d has no Contents", n)
		}
		refs := strings.Trim(string(cm[1]), "[] \n")
		obj = reContents.ReplaceAll(obj, fmt.Appendf(nil, "/Contents [%d 0 R %s]", extraNum, refs))

		boxes := fmt.Sprintf("/MediaBox [0 0 %s %s]", num(w), num(h))
		if trim > 0 {
			boxes += fmt.Sprintf("\n/BleedBox [0 0 %s %s]\n/TrimBox [%s %s %s %s]",
				num(w), num(h), num(trim), num(trim), num(w-trim), num(h-trim))
		}
		obj = reMediaBox.ReplaceAll(obj, []byte(boxes))
		body[n] = obj
	}
	if pages == 0 {
		return nil, errors.New("no pages found")
	}

	// Write it back out: same objects in the same order, new ones appended,
	// one fresh xref table.
	var out bytes.Buffer
	out.Write(data[:offsets[nums[0]]])
	newOff := map[int]int{}
	for _, n := range nums {
		newOff[n] = out.Len()
		out.Write(body[n])
		out.WriteString("\n")
	}
	for i, o := range extra {
		newOff[size+i] = out.Len()
		out.Write(o)
		out.WriteString("\n")
	}
	total := size + len(extra)
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", total)
	for n := 1; n < total; n++ {
		if off, ok := newOff[n]; ok {
			fmt.Fprintf(&out, "%010d 00000 n \n", off)
		} else {
			out.WriteString("0000000000 65535 f \n")
		}
	}
	trailer := reSize.ReplaceAll(tm[1], fmt.Appendf(nil, "/Size %d", total))
	fmt.Fprintf(&out, "trailer\n<<%s>>\nstartxref\n%d\n%%%%EOF\n", trailer, xref)
	return out.Bytes(), nil
}

// num formats a PDF number: up to 4 decimals, no trailing zeros.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		return "0"
	}
	return s
}
