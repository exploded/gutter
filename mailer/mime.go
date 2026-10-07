package mailer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"time"
)

// BuildMessage renders an RFC 5322 message: a text/HTML alternative, wrapped in
// multipart/mixed when there are attachments. from, to and replyTo are header
// values, already formatted (see net/mail.Address.String). It is the raw form
// the Gmail API sends; SES builds its own.
func BuildMessage(from, to, replyTo, subject, htmlBody, textBody string, atts ...Attachment) ([]byte, error) {
	var buf bytes.Buffer
	hdr := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	hdr("From", from)
	hdr("To", to)
	if replyTo != "" {
		hdr("Reply-To", replyTo)
	}
	hdr("Subject", mime.QEncoding.Encode("utf-8", subject))
	hdr("Date", time.Now().Format(time.RFC1123Z))
	hdr("MIME-Version", "1.0")

	alt, err := alternative(textBody, htmlBody)
	if err != nil {
		return nil, err
	}
	if len(atts) == 0 {
		hdr("Content-Type", alt.contentType)
		buf.WriteString("\r\n")
		buf.Write(alt.body)
		return buf.Bytes(), nil
	}

	var body bytes.Buffer
	mixed := multipart.NewWriter(&body)
	hdr("Content-Type", "multipart/mixed; boundary="+mixed.Boundary())
	buf.WriteString("\r\n")

	w, err := mixed.CreatePart(textproto.MIMEHeader{"Content-Type": {alt.contentType}})
	if err != nil {
		return nil, err
	}
	w.Write(alt.body)
	for _, a := range atts {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		w, err := mixed.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {mime.FormatMediaType(ct, map[string]string{"name": a.Filename})},
			"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename})},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return nil, err
		}
		writeBase64(w, a.Data)
	}
	if err := mixed.Close(); err != nil {
		return nil, err
	}
	buf.Write(body.Bytes())
	return buf.Bytes(), nil
}

type part struct {
	contentType string
	body        []byte
}

// alternative builds the multipart/alternative body. Text comes first: mail
// clients show the last part they understand.
func alternative(textBody, htmlBody string) (part, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, p := range []struct{ ct, s string }{
		{"text/plain; charset=utf-8", textBody},
		{"text/html; charset=utf-8", htmlBody},
	} {
		if p.s == "" {
			continue
		}
		w, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {p.ct},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return part{}, err
		}
		writeBase64(w, []byte(p.s))
	}
	if err := mw.Close(); err != nil {
		return part{}, err
	}
	return part{contentType: "multipart/alternative; boundary=" + mw.Boundary(), body: body.Bytes()}, nil
}

// writeBase64 writes data as base64 in 76-character lines, as RFC 2045 requires.
func writeBase64(w io.Writer, data []byte) {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		w.Write([]byte(enc[:76] + "\r\n"))
		enc = enc[76:]
	}
	w.Write([]byte(enc + "\r\n"))
}
