package mailer

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

// The message must parse back into the same text, HTML and attachment bytes:
// a mangled PDF arrives the right size but blank.
func TestBuildMessageRoundTrip(t *testing.T) {
	pdf := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte{0x00, 0xff, 0x80, '\n'}, 100)...)
	raw, err := BuildMessage(`"Up The Spout" <vin@example.test>`, "ann@example.test", "", "Invoice — $279.00",
		"<p>Thanks Ann — paid.</p>", "Thanks Ann — paid.",
		Attachment{Filename: "INV-1001.pdf", ContentType: "application/pdf", Data: pdf})
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if subj, _ := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject")); subj != "Invoice — $279.00" {
		t.Errorf("Subject = %q", subj)
	}
	if m.Header.Get("Reply-To") != "" {
		t.Error("empty replyTo still wrote a Reply-To header")
	}

	parts := readParts(t, m.Header.Get("Content-Type"), m.Body)
	if got := parts["text/plain"]; got != "Thanks Ann — paid." {
		t.Errorf("text part = %q", got)
	}
	if got := parts["text/html"]; got != "<p>Thanks Ann — paid.</p>" {
		t.Errorf("html part = %q", got)
	}
	if got := parts["application/pdf"]; got != string(pdf) {
		t.Error("PDF attachment did not survive the round trip")
	}
	if !strings.Contains(parts["disposition:application/pdf"], `filename=INV-1001.pdf`) {
		t.Errorf("attachment disposition = %q", parts["disposition:application/pdf"])
	}
}

// Without attachments the top level is the alternative itself.
func TestBuildMessageNoAttachments(t *testing.T) {
	raw, err := BuildMessage("a@example.test", "b@example.test", "c@example.test", "Hi", "<p>hi</p>", "hi")
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if mt, _, _ := mime.ParseMediaType(m.Header.Get("Content-Type")); mt != "multipart/alternative" {
		t.Errorf("Content-Type = %q, want multipart/alternative", mt)
	}
	if m.Header.Get("Reply-To") != "c@example.test" {
		t.Errorf("Reply-To = %q", m.Header.Get("Reply-To"))
	}
}

// readParts walks a multipart tree and returns each leaf's decoded body keyed
// by media type, plus "disposition:<type>" for its Content-Disposition.
func readParts(t *testing.T, contentType string, body io.Reader) map[string]string {
	t.Helper()
	out := map[string]string{}
	mt, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(mt, "multipart/") {
		t.Fatalf("not multipart: %s", mt)
	}
	mr := multipart.NewReader(body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		ct := p.Header.Get("Content-Type")
		pt, _, _ := mime.ParseMediaType(ct)
		if strings.HasPrefix(pt, "multipart/") {
			for k, v := range readParts(t, ct, p) {
				out[k] = v
			}
			continue
		}
		data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
		if err != nil {
			t.Fatalf("%s: %v", pt, err)
		}
		out[pt] = string(data)
		out["disposition:"+pt] = p.Header.Get("Content-Disposition")
	}
}
