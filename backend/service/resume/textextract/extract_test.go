package textextract_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/service/resume/filetype"
	"github.com/trushilpatel/jobradar/service/resume/textextract"
)

// erroringReader always fails — used to exercise ExtractText's io.ReadAll
// error branch, which a well-formed in-memory reader can never trigger.
type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

// buildMalformedContentPDF assembles a byte-exact minimal PDF (one page,
// hand-computed xref offsets) whose content stream is the single token "Tf"
// — a text-font operator missing its two required operands. The pdf
// library's operator dispatch panics ("bad TL") on this malformed stream;
// GetPlainText recovers the panic into a returned error, which is exactly
// the extractPDF error branch this exercises.
func buildMalformedContentPDF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	var offsets [5]int

	buf.WriteString("%PDF-1.4\n")

	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	offsets[3] = buf.Len()
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /Resources << >> /MediaBox [0 0 200 200] /Contents 4 0 R >>\nendobj\n")

	offsets[4] = buf.Len()
	content := "Tf"
	fmt.Fprintf(&buf, "4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content)

	xrefStart := buf.Len()
	buf.WriteString("xref\n0 5\n")
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", xrefStart)

	return buf.Bytes()
}

// buildTestDocx assembles a minimal valid docx zip in memory: the two
// entries docx.ReadDocxFromMemory actually requires
// (word/document.xml + word/_rels/document.xml.rels) — anything less and the
// library itself errors before textextract ever sees the content.
func buildTestDocx(t *testing.T, documentXML string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	writeEntry := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	writeEntry("word/document.xml", documentXML)
	writeEntry("word/_rels/document.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`)

	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func docxParagraph(text string) string {
	return `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` +
		text + `</w:t></w:r></w:p></w:body></w:document>`
}

func TestExtractText_PDF_Success(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	text, err := textextract.ExtractText(filetype.ContentTypePDF, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "This is a heading") {
		t.Fatalf("expected extracted text to contain fixture content, got %q", text)
	}
	if len(text) < textextract.MinTextLength {
		t.Fatalf("expected extracted text to clear MinTextLength, got %d chars", len(text))
	}
}

func TestExtractText_PDF_CorruptBytesReturnsError(t *testing.T) {
	_, err := textextract.ExtractText(filetype.ContentTypePDF, strings.NewReader("not a real pdf"))
	if err == nil {
		t.Fatal("expected an error for corrupt PDF bytes")
	}
}

func TestExtractText_PDF_MalformedContentStreamReturnsError(t *testing.T) {
	data := buildMalformedContentPDF(t)

	_, err := textextract.ExtractText(filetype.ContentTypePDF, bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected an error when the PDF content stream panics during interpretation")
	}
}

func TestExtractText_ReadInputErrorPropagates(t *testing.T) {
	_, err := textextract.ExtractText(filetype.ContentTypePDF, erroringReader{})
	if err == nil {
		t.Fatal("expected an error when the input reader itself fails")
	}
}

func TestExtractText_PDF_EmptyReaderReturnsError(t *testing.T) {
	_, err := textextract.ExtractText(filetype.ContentTypePDF, bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected an error for an empty PDF reader")
	}
}

func TestExtractText_DOCX_Success(t *testing.T) {
	longText := "This is a sufficiently long resume paragraph used to clear the short-content threshold for testing purposes."
	data := buildTestDocx(t, docxParagraph(longText))

	text, err := textextract.ExtractText(filetype.ContentTypeDOCX, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "This is a sufficiently long resume paragraph") {
		t.Fatalf("expected plain text without XML tags, got %q", text)
	}
	if strings.Contains(text, "<w:") {
		t.Fatalf("expected XML tags to be stripped, got %q", text)
	}
}

func TestExtractText_DOCX_UnescapesXMLEntities(t *testing.T) {
	data := buildTestDocx(t, docxParagraph("Go &amp; Postgres &amp; distributed systems experience with 5+ years background"))

	text, err := textextract.ExtractText(filetype.ContentTypeDOCX, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Go & Postgres & distributed systems") {
		t.Fatalf("expected unescaped ampersands, got %q", text)
	}
}

func TestExtractText_DOCX_CorruptBytesReturnsError(t *testing.T) {
	_, err := textextract.ExtractText(filetype.ContentTypeDOCX, strings.NewReader("not a zip file"))
	if err == nil {
		t.Fatal("expected an error for corrupt DOCX bytes")
	}
}

func TestExtractText_DOCX_MissingDocumentXMLReturnsError(t *testing.T) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	w, err := zw.Create("word/_rels/document.xml.rels")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	_, _ = w.Write([]byte("<Relationships/>"))
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}

	_, err = textextract.ExtractText(filetype.ContentTypeDOCX, bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Fatal("expected an error when word/document.xml is missing")
	}
}

func TestExtractText_UnsupportedContentTypeReturnsError(t *testing.T) {
	_, err := textextract.ExtractText("text/plain", strings.NewReader("hello"))
	if !errors.Is(err, textextract.ErrUnsupportedContentType) {
		t.Fatalf("expected ErrUnsupportedContentType, got %v", err)
	}
}

func TestExtractText_ShortContentReturnsSentinelError(t *testing.T) {
	data := buildTestDocx(t, docxParagraph("Hi"))

	_, err := textextract.ExtractText(filetype.ContentTypeDOCX, bytes.NewReader(data))
	if !errors.Is(err, textextract.ErrShortContent) {
		t.Fatalf("expected ErrShortContent for near-empty text, got %v", err)
	}
}

func TestExtractText_WhitespaceOnlyContentReturnsShortContentError(t *testing.T) {
	data := buildTestDocx(t, docxParagraph("                                                                     "))

	_, err := textextract.ExtractText(filetype.ContentTypeDOCX, bytes.NewReader(data))
	if !errors.Is(err, textextract.ErrShortContent) {
		t.Fatalf("expected ErrShortContent for whitespace-only text, got %v", err)
	}
}

func TestExtractText_TrimsSurroundingWhitespace(t *testing.T) {
	inner := "Backend engineer with extensive Go, PostgreSQL, and distributed systems experience across five roles."
	data := buildTestDocx(t, docxParagraph("   "+inner+"   "))

	text, err := textextract.ExtractText(filetype.ContentTypeDOCX, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") {
		t.Fatalf("expected surrounding whitespace to be trimmed, got %q", text)
	}
}

func TestExtractText_CollapsesEmbeddedNewlinesForMultiWordMatching(t *testing.T) {
	inner := "Skilled in machine\nlearning and distributed systems with five years of experience."
	data := buildTestDocx(t, docxParagraph(inner))

	text, err := textextract.ExtractText(filetype.ContentTypeDOCX, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if strings.Contains(text, "\n") {
		t.Fatalf("expected no embedded newlines in extracted text, got %q", text)
	}
	if !strings.Contains(text, "machine learning") {
		t.Fatalf("expected the newline-broken phrase to join into one matchable string, got %q", text)
	}
}
