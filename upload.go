package blazingagents

import (
	"bytes"
	"cmp"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
)

// Upload is a file sent in a multipart request.
type Upload struct {
	name        string
	contentType string
	open        func() (io.ReadCloser, error)
}

// UploadFile sends the file at path, named by its base name. The call opens it before
// sending; a file that does not open fails the call with an *ArgumentError.
func UploadFile(path string) Upload {
	return Upload{name: filepath.Base(path), open: func() (io.ReadCloser, error) { return os.Open(path) }}
}

// UploadBytes sends b as a file named name.
func UploadBytes(name string, b []byte) Upload {
	return Upload{name: name, open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }}
}

// UploadReader sends what r yields as a file named name. r is read once, and closed after
// sending when it is an io.Closer.
func UploadReader(name string, r io.Reader) Upload {
	return Upload{name: name, open: func() (io.ReadCloser, error) {
		if rc, ok := r.(io.ReadCloser); ok {
			return rc, nil
		}
		return io.NopCloser(r), nil
	}}
}

// WithContentType sets the part's content type instead of guessing it from the name.
func (u Upload) WithContentType(ct string) Upload {
	u.contentType = ct
	return u
}

// contentTypes is keyed by the last lower-cased extension. mime.TypeByExtension is not used,
// because its answer depends on the host.
var contentTypes = map[string]string{
	"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg", "webp": "image/webp", "gif": "image/gif",
	"zip": "application/zip", "tar": "application/x-tar", "gz": "application/gzip", "tgz": "application/gzip",
	"md": "text/markdown", "txt": "text/plain", "json": "application/json", "pdf": "application/pdf",
}

func (u Upload) partContentType(name string) string {
	if u.contentType != "" {
		return u.contentType
	}
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		if ct, ok := contentTypes[strings.ToLower(name[i+1:])]; ok {
			return ct
		}
	}
	return "application/octet-stream"
}

// multipartParams is the body of uploadAgentAvatar and uploadSkill.
type multipartParams struct {
	fields      [][2]string
	file        Upload
	defaultName string // used when the Upload has no name
}

// binaryParams is the body of putSkillFile.
type binaryParams struct {
	r    io.Reader
	Path string `query:"path"`
}

func encodeMultipart(p multipartParams) (io.Reader, string, []string, error) {
	if p.file.open == nil {
		return nil, "", nil, &ArgumentError{Name: "file", Reason: "zero Upload"}
	}
	rc, err := p.file.open()
	if err != nil {
		return nil, "", nil, &ArgumentError{Name: "file", Reason: err.Error(), Err: err}
	}
	name := cmp.Or(p.file.name, p.defaultName)
	pr, pw := io.Pipe()
	sw := &stickyWriter{w: pw}
	mw := multipart.NewWriter(sw)
	go func() {
		for _, f := range p.fields {
			_ = mw.WriteField(f[0], f[1])
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": name}))
		h.Set("Content-Type", p.file.partContentType(name))
		part, _ := mw.CreatePart(h)
		_, err := io.Copy(part, rc)
		rc.Close() // before the body ends, so the reader is closed once the request is sent
		_ = mw.Close()
		pw.CloseWithError(cmp.Or(err, sw.err))
	}()
	return pr, mw.FormDataContentType(), nil, nil
}

// stickyWriter keeps the first write error and swallows the rest, so the multipart writer
// never fails mid-part; the error ends the pipe once the part is written.
type stickyWriter struct {
	w   io.Writer
	err error
}

func (s *stickyWriter) Write(p []byte) (int, error) {
	if s.err == nil {
		_, s.err = s.w.Write(p)
	}
	return len(p), nil
}
