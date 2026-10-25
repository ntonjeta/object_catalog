package httpapi

import (
	"net/http"
	"time"

	"biblio-oggetto/internal/domain"
)

type categoryRequest struct {
	Name string `json:"name"`
}

type categoryResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type categoryEnvelope struct {
	Category categoryResponse `json:"category"`
}

type categoriesEnvelope struct {
	Categories []categoryResponse `json:"categories"`
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.categories.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	response := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		response = append(response, toCategoryResponse(category))
	}
	writeJSON(w, http.StatusOK, categoriesEnvelope{Categories: response})
}

func (s *Server) getCategory(w http.ResponseWriter, r *http.Request) {
	category, err := s.categories.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, categoryEnvelope{Category: toCategoryResponse(category)})
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	var request categoryRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	category, err := s.categories.Create(r.Context(), request.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, categoryEnvelope{Category: toCategoryResponse(category)})
}

func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request) {
	var request categoryRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	category, err := s.categories.Update(r.Context(), r.PathValue("id"), request.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, categoryEnvelope{Category: toCategoryResponse(category)})
}

func (s *Server) deleteCategory(w http.ResponseWriter, r *http.Request) {
	if err := s.categories.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toCategoryResponse(category domain.Category) categoryResponse {
	return categoryResponse{
		ID:        category.ID,
		Name:      category.Name,
		CreatedAt: category.CreatedAt,
		UpdatedAt: category.UpdatedAt,
	}
}
