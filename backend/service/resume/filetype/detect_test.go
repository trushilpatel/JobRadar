package filetype

import (
	"archive/zip"
	"bytes"
	"testing"
)

func validDocxBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"[Content_Types].xml", "word/document.xml"} {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := f.Write([]byte("<xml/>")); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func otherOoxmlBytes(t *testing.T) []byte {
	// A ZIP with [Content_Types].xml but NOT word/document.xml — e.g. an .xlsx.
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("[Content_Types].xml")
	_, _ = f.Write([]byte("<xml/>"))
	f2, _ := zw.Create("xl/workbook.xml")
	_, _ = f2.Write([]byte("<xml/>"))
	_ = zw.Close()
	return buf.Bytes()
}

func TestDetect_EmptyFile(t *testing.T) {
	if _, _, err := Detect(nil); err != ErrEmpty {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}

func TestDetect_ValidPDFHeader(t *testing.T) {
	ct, ext, err := Detect([]byte("%PDF-1.4\n...rest of file..."))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ct != ContentTypePDF || ext != "pdf" {
		t.Fatalf("got (%s, %s), want (%s, pdf)", ct, ext, ContentTypePDF)
	}
}

func TestDetect_ValidDocx(t *testing.T) {
	ct, ext, err := Detect(validDocxBytes(t))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ct != ContentTypeDOCX || ext != "docx" {
		t.Fatalf("got (%s, %s), want (%s, docx)", ct, ext, ContentTypeDOCX)
	}
}

func TestDetect_OtherOoxmlFormatRejected(t *testing.T) {
	// Guards against accepting .xlsx/.pptx just because they're ZIPs too.
	if _, _, err := Detect(otherOoxmlBytes(t)); err != ErrUnsupportedType {
		t.Fatalf("expected ErrUnsupportedType for non-Word OOXML, got %v", err)
	}
}

func TestDetect_RenamedExecutableRejected(t *testing.T) {
	if _, _, err := Detect([]byte("MZ\x90\x00\x03\x00\x00\x00")); err != ErrUnsupportedType {
		t.Fatalf("expected ErrUnsupportedType for exe magic bytes, got %v", err)
	}
}

func TestDetect_TruncatedZipRejected(t *testing.T) {
	// Truncated after the local file header signature — not a valid zip.
	if _, _, err := Detect([]byte("PK\x03\x04truncated")); err != ErrUnsupportedType {
		t.Fatalf("expected ErrUnsupportedType for truncated zip, got %v", err)
	}
}

func TestDetect_CorruptButSignatureValidPDFIsAccepted(t *testing.T) {
	// Phase 0 does no full-document parse — a truncated-but-header-valid PDF
	// is accepted at upload time (documented limitation, not a bug).
	ct, _, err := Detect([]byte("%PDF-1.4"))
	if err != nil || ct != ContentTypePDF {
		t.Fatalf("expected a bare PDF header to be accepted, got ct=%s err=%v", ct, err)
	}
}
