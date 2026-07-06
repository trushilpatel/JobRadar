// Package filetype detects whether a byte slice is a PDF or a DOCX by
// inspecting its content, never trusting a declared extension or MIME type.
package filetype

import (
	"archive/zip"
	"bytes"
	"errors"
)

// ErrUnsupportedType is returned when the bytes match neither PDF nor DOCX.
var ErrUnsupportedType = errors.New("filetype: unsupported or unrecognized file type")

// ErrEmpty is returned for a zero-length input.
var ErrEmpty = errors.New("filetype: empty file")

const (
	ContentTypePDF  = "application/pdf"
	ContentTypeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

// Detect sniffs data and returns the content type + extension. It rejects
// anything that isn't a PDF (by header) or a genuine Word DOCX (a ZIP that
// contains both "[Content_Types].xml" and "word/document.xml" — this
// specifically excludes other OOXML formats like .xlsx/.pptx, which are also
// ZIPs but lack word/document.xml).
func Detect(data []byte) (contentType, extension string, err error) {
	if len(data) == 0 {
		return "", "", ErrEmpty
	}
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return ContentTypePDF, "pdf", nil
	}
	if looksLikeDocx(data) {
		return ContentTypeDOCX, "docx", nil
	}
	return "", "", ErrUnsupportedType
}

func looksLikeDocx(data []byte) bool {
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return false
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	hasContentTypes, hasWordDocument := false, false
	for _, f := range zr.File {
		switch f.Name {
		case "[Content_Types].xml":
			hasContentTypes = true
		case "word/document.xml":
			hasWordDocument = true
		}
	}
	return hasContentTypes && hasWordDocument
}
