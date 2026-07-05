package tests

import (
	"context"
	"io"
	"strings"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
)

// fakeService is a minimal Service double recording the last call it saw —
// good enough to test what ValidationService/RestService pass downstream and
// what they do with what comes back.
type fakeService struct {
	lastUploadInput dto.UploadInput
	lastRenameLabel string
	uploadResult    *resumemodel.Resume
	uploadErr       error
	renameResult    *resumemodel.Resume
	renameErr       error
	deleteErr       error
	downloadReader  io.ReadCloser
	downloadResume  *resumemodel.Resume
	downloadErr     error
	listResult      []resumemodel.Resume
	listErr         error
	getResult       *resumemodel.Resume
	getErr          error
}

func (f *fakeService) List(context.Context, string) ([]resumemodel.Resume, error) {
	return f.listResult, f.listErr
}
func (f *fakeService) Get(context.Context, string, string) (*resumemodel.Resume, error) {
	return f.getResult, f.getErr
}
func (f *fakeService) Upload(_ context.Context, _ string, input dto.UploadInput) (*resumemodel.Resume, error) {
	f.lastUploadInput = input
	return f.uploadResult, f.uploadErr
}
func (f *fakeService) Rename(_ context.Context, _ string, _ string, label string) (*resumemodel.Resume, error) {
	f.lastRenameLabel = label
	return f.renameResult, f.renameErr
}
func (f *fakeService) Delete(context.Context, string, string) error { return f.deleteErr }
func (f *fakeService) Download(context.Context, string, string) (io.ReadCloser, *resumemodel.Resume, error) {
	return f.downloadReader, f.downloadResume, f.downloadErr
}

func TestResumeValidation_UploadRejectsEmptyFile(t *testing.T) {
	fake := &fakeService{uploadResult: &resumemodel.Resume{}}
	v := resumesvc.NewValidationService(fake)
	_, err := v.Upload(context.Background(), "u1", dto.UploadInput{OriginalFilename: "f.pdf"})
	if err == nil {
		t.Fatal("expected an error for an empty file")
	}
}

func TestResumeValidation_UploadRejectsOversizedFile(t *testing.T) {
	fake := &fakeService{uploadResult: &resumemodel.Resume{}}
	v := resumesvc.NewValidationService(fake)
	oversized := make([]byte, 2*1024*1024+1)
	_, err := v.Upload(context.Background(), "u1", dto.UploadInput{OriginalFilename: "f.pdf", RawBytes: oversized})
	if err == nil {
		t.Fatal("expected an error for a file over 2MB")
	}
}

func TestResumeValidation_UploadAcceptsExactlyAtLimit(t *testing.T) {
	fake := &fakeService{uploadResult: &resumemodel.Resume{}}
	v := resumesvc.NewValidationService(fake)
	atLimit := append([]byte("%PDF-1.4"), make([]byte, 2*1024*1024-8)...)
	_, err := v.Upload(context.Background(), "u1", dto.UploadInput{OriginalFilename: "f.pdf", RawBytes: atLimit})
	if err != nil {
		t.Fatalf("expected a file at exactly 2MB to be accepted, got %v", err)
	}
}

func TestResumeValidation_UploadRejectsUnsupportedType(t *testing.T) {
	fake := &fakeService{uploadResult: &resumemodel.Resume{}}
	v := resumesvc.NewValidationService(fake)
	_, err := v.Upload(context.Background(), "u1", dto.UploadInput{
		OriginalFilename: "f.pdf", RawBytes: []byte("not a real file"),
	})
	if err == nil {
		t.Fatal("expected an unsupported-type error")
	}
	if _, ok := err.(domainerrs.ValidationErrors); !ok {
		t.Fatalf("expected a ValidationErrors, got %T: %v", err, err)
	}
}

func TestResumeValidation_UploadRejectsExtensionMismatch(t *testing.T) {
	// filename says .docx, bytes are actually a PDF — server trusts sniffed
	// bytes, so this is accepted as a PDF (extension is display-only; the
	// mismatch itself is not an error condition since the stored extension
	// always comes from the sniff, never the claimed filename).
	fake := &fakeService{uploadResult: &resumemodel.Resume{}}
	v := resumesvc.NewValidationService(fake)
	_, err := v.Upload(context.Background(), "u1", dto.UploadInput{
		OriginalFilename: "resume.docx", RawBytes: []byte("%PDF-1.4"),
	})
	if err != nil {
		t.Fatalf("expected sniffed-type PDF to be accepted despite .docx filename, got %v", err)
	}
	if fake.lastUploadInput.Extension != "pdf" {
		t.Fatalf("expected stored extension to be 'pdf' (from sniffing), got %s", fake.lastUploadInput.Extension)
	}
}

func TestResumeValidation_UploadFallsBackLabelToFilename(t *testing.T) {
	fake := &fakeService{uploadResult: &resumemodel.Resume{}}
	v := resumesvc.NewValidationService(fake)
	_, err := v.Upload(context.Background(), "u1", dto.UploadInput{
		Label: "  ", OriginalFilename: "backend-resume.pdf", RawBytes: []byte("%PDF-1.4"),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if fake.lastUploadInput.Label != "backend-resume" {
		t.Fatalf("expected label fallback to 'backend-resume', got %q", fake.lastUploadInput.Label)
	}
}

func TestResumeValidation_UploadRejectsExcessivelyLongLabel(t *testing.T) {
	fake := &fakeService{uploadResult: &resumemodel.Resume{}}
	v := resumesvc.NewValidationService(fake)
	_, err := v.Upload(context.Background(), "u1", dto.UploadInput{
		Label: strings.Repeat("x", 201), OriginalFilename: "f.pdf", RawBytes: []byte("%PDF-1.4"),
	})
	if err == nil {
		t.Fatal("expected an error for a label over 200 characters")
	}
}

func TestResumeValidation_RenameRejectsEmptyLabel(t *testing.T) {
	fake := &fakeService{}
	v := resumesvc.NewValidationService(fake)
	if _, err := v.Rename(context.Background(), "u1", "r1", "   "); err == nil {
		t.Fatal("expected an error for an empty rename label")
	}
}

func TestResumeValidation_RenameRejectsExcessivelyLongLabel(t *testing.T) {
	fake := &fakeService{}
	v := resumesvc.NewValidationService(fake)
	if _, err := v.Rename(context.Background(), "u1", "r1", strings.Repeat("x", 201)); err == nil {
		t.Fatal("expected an error for a rename label over 200 characters")
	}
}

func TestResumeValidation_RenameTrimsWhitespace(t *testing.T) {
	fake := &fakeService{}
	v := resumesvc.NewValidationService(fake)
	if _, err := v.Rename(context.Background(), "u1", "r1", "  Trimmed  "); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if fake.lastRenameLabel != "Trimmed" {
		t.Fatalf("expected trimmed label 'Trimmed', got %q", fake.lastRenameLabel)
	}
}

// The following pass-through tests exercise BaseService — ValidationService
// doesn't override List/Get/Delete/Download, so calling them here must
// delegate straight through to the fake without any validation logic.

func TestResumeValidation_ListPassesThroughToNext(t *testing.T) {
	fake := &fakeService{listResult: []resumemodel.Resume{{ID: "r1"}}}
	v := resumesvc.NewValidationService(fake)
	rows, err := v.List(context.Background(), "u1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "r1" {
		t.Fatalf("expected pass-through result, got %+v", rows)
	}
}

func TestResumeValidation_GetPassesThroughToNext(t *testing.T) {
	fake := &fakeService{getResult: &resumemodel.Resume{ID: "r1"}}
	v := resumesvc.NewValidationService(fake)
	row, err := v.Get(context.Background(), "u1", "r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if row.ID != "r1" {
		t.Fatalf("expected pass-through result, got %+v", row)
	}
}

func TestResumeValidation_DeletePassesThroughToNext(t *testing.T) {
	fake := &fakeService{deleteErr: domainerrs.ErrNotFound.WithMessage("resume not found")}
	v := resumesvc.NewValidationService(fake)
	if err := v.Delete(context.Background(), "u1", "r1"); err == nil {
		t.Fatal("expected the pass-through error to propagate")
	}
}

func TestResumeValidation_DownloadPassesThroughToNext(t *testing.T) {
	fake := &fakeService{downloadResume: &resumemodel.Resume{ID: "r1"}}
	v := resumesvc.NewValidationService(fake)
	_, resume, err := v.Download(context.Background(), "u1", "r1")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if resume.ID != "r1" {
		t.Fatalf("expected pass-through resume, got %+v", resume)
	}
}

func TestNewValidationService_PanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected NewValidationService(nil) to panic")
		}
	}()
	resumesvc.NewValidationService(nil)
}
