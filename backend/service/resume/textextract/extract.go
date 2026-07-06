// Package textextract converts an uploaded resume's raw bytes into plain
// text so it can be handed to an LLM for keyword extraction. It is
// deliberately narrow: no block/structure model, just a flat string,
// best-effort. A near-empty extraction result (e.g. a scanned image PDF with
// no text layer) is reported as ErrShortContent rather than silently
// returned — callers must never feed near-empty text into an LLM call.
package textextract

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/nguyenthenguyen/docx"
	"github.com/trushilpatel/jobradar/service/resume/filetype"
)

// ErrUnsupportedContentType is returned when contentType is neither of the
// two values allowed by the `resumes.content_type` CHECK constraint
// (migration 086): application/pdf or the DOCX OOXML MIME type.
var ErrUnsupportedContentType = errors.New("textextract: unsupported content type")

// ErrShortContent is returned when the extracted text falls below
// MinTextLength — too short to be a meaningful resume body. This is the
// sentinel callers check to skip the downstream LLM call rather than
// spending a call on garbage input.
var ErrShortContent = errors.New("textextract: extracted text too short")

// MinTextLength is the minimum trimmed-text length (in runes, checked via
// byte length which is a safe lower bound for any UTF-8 text) considered a
// meaningful extraction result. Below this, ExtractText returns
// ErrShortContent instead of the text.
const MinTextLength = 50

// ExtractText dispatches on contentType and returns the best-effort plain
// text of r's contents. r is fully buffered (both underlying libraries
// require io.ReaderAt, and resumes are capped at 2MB — see
// resumecommon.MaxFileSizeBytes — so buffering is bounded and cheap).
func ExtractText(contentType string, r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("textextract: read input: %w", err)
	}

	var text string
	switch contentType {
	case filetype.ContentTypePDF:
		text, err = extractPDF(data)
	case filetype.ContentTypeDOCX:
		text, err = extractDOCX(data)
	default:
		return "", ErrUnsupportedContentType
	}
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(collapseWhitespace(text))
	if len(trimmed) < MinTextLength {
		return "", ErrShortContent
	}
	return trimmed, nil
}

func extractPDF(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("textextract: open pdf: %w", err)
	}
	plain, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("textextract: pdf plain text: %w", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(plain); err != nil {
		return "", fmt.Errorf("textextract: read pdf text: %w", err)
	}
	return buf.String(), nil
}

// docxBreakTag matches the line/tab break tags the docx library documents as
// NEWLINE/TAB constants — replaced with a space before tag-stripping so words
// either side of a break never get glued together.
var docxBreakTag = regexp.MustCompile(`<w:(br|tab|cr)\s*/>`)

// docxTag matches any remaining XML/OOXML tag once break tags are handled.
var docxTag = regexp.MustCompile(`<[^>]+>`)

// docxWhitespace collapses runs of whitespace left behind by tag stripping.
var docxWhitespace = regexp.MustCompile(`[ \t]+`)

func extractDOCX(data []byte) (string, error) {
	r, err := docx.ReadDocxFromMemory(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("textextract: open docx: %w", err)
	}
	defer func() { _ = r.Close() }()

	// GetContent returns the raw word/document.xml markup (the library is
	// built for find/replace on that XML, not plain-text extraction) — tags
	// must be stripped and entities unescaped to recover readable text.
	raw := r.Editable().GetContent()
	withBreaks := docxBreakTag.ReplaceAllString(raw, " ")
	stripped := docxTag.ReplaceAllString(withBreaks, " ")
	unescaped := html.UnescapeString(stripped)
	return docxWhitespace.ReplaceAllString(unescaped, " "), nil
}

// allWhitespace collapses any run of whitespace — spaces, tabs, newlines —
// into a single space. The sole consumer of ExtractText's output is
// client-side keyword matching against a job's keywords, which needs one
// clean string rather than preserved paragraph structure; a stray line break
// from PDF line-wrapping or DOCX markup must never split a multi-word
// keyword like "machine learning" across an unmatched newline.
var allWhitespace = regexp.MustCompile(`\s+`)

// collapseWhitespace normalizes every run of whitespace (including newlines)
// to a single space.
func collapseWhitespace(s string) string {
	return allWhitespace.ReplaceAllString(s, " ")
}
