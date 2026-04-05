package handler

import (
	"net/http"

	"github.com/trushilpatel/jobradar/internal/store"
)

type CompanyHandler struct {
	companies store.CompanyReader
}

func NewCompanyHandler(companies store.CompanyReader) *CompanyHandler {
	return &CompanyHandler{companies: companies}
}

// GET /companies
func (h *CompanyHandler) List(w http.ResponseWriter, r *http.Request) {
	companies, err := h.companies.GetActiveCompanies()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch companies")
		return
	}

	respond(w, http.StatusOK, companies)
}
