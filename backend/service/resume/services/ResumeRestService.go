package services

import (
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/service/resume/dto"
)

// multipartMaxMemory bounds how much of the multipart body chi/net/http
// buffers in memory before spilling to disk. Set comfortably above the 2MB
// file cap plus form-field/boundary overhead.
const multipartMaxMemory = 4 << 20

type RestService struct {
	next Service
}

func NewRestService(next Service) *RestService {
	if next == nil {
		panic("resume.NewRestService: next must not be nil")
	}
	return &RestService{next: next}
}

func (h *RestService) list(w http.ResponseWriter, r *http.Request) {
	userID := identity.MustFromContext(r.Context()).UserID
	resumes, err := h.next.List(r.Context(), userID)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, resumes)
}

func (h *RestService) upload(w http.ResponseWriter, r *http.Request) {
	userID := identity.MustFromContext(r.Context()).UserID

	if err := r.ParseMultipartForm(multipartMaxMemory); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid multipart form")
		return
	}
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "file is required")
		return
	}
	defer file.Close()

	data, readErr := io.ReadAll(file)
	if readErr != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "failed to read uploaded file")
		return
	}

	input := dto.UploadInput{
		Label:            r.FormValue("label"),
		OriginalFilename: fileHeader.Filename,
		RawBytes:         data,
		DeclaredSize:     int64(len(data)),
	}

	resume, uploadErr := h.next.Upload(r.Context(), userID, input)
	if uploadErr != nil {
		handler.HandleError(w, uploadErr)
		return
	}
	handler.Respond(w, http.StatusCreated, resume)
}

func (h *RestService) download(w http.ResponseWriter, r *http.Request) {
	userID := identity.MustFromContext(r.Context()).UserID
	id := chi.URLParam(r, "id")

	reader, resume, err := h.next.Download(r.Context(), userID, id)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", resume.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": resume.OriginalFilename}))
	w.Header().Set("Content-Length", strconv.FormatInt(resume.SizeBytes, 10))
	w.WriteHeader(http.StatusOK)
	// Headers are already committed at this point — an io.Copy failure here
	// (client disconnect, storage read timeout) cannot change the response
	// status, so this deliberately bypasses handler.Respond/HandleError.
	// Nothing else to do but let the partial transfer end.
	_, _ = io.Copy(w, reader)
}

type renameRequest struct {
	Label string `json:"label"`
}

func (h *RestService) rename(w http.ResponseWriter, r *http.Request) {
	userID := identity.MustFromContext(r.Context()).UserID
	id := chi.URLParam(r, "id")

	req, ok := handler.DecodeBody[renameRequest](w, r)
	if !ok {
		return
	}
	resume, err := h.next.Rename(r.Context(), userID, id, req.Label)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, resume)
}

func (h *RestService) remove(w http.ResponseWriter, r *http.Request) {
	userID := identity.MustFromContext(r.Context()).UserID
	id := chi.URLParam(r, "id")

	if err := h.next.Delete(r.Context(), userID, id); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.RespondNoContent(w)
}
