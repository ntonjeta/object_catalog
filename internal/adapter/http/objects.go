package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

const imageURLPrefix = "/static/uploads/"

type createObjectRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	CategoryID  string   `json:"category_id"`
	Tags        []string `json:"tags"`
}

type updateObjectRequest struct {
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	CategoryID  *string   `json:"category_id"`
	Tags        *[]string `json:"tags"`
	RemovePhoto *bool     `json:"remove_photo"`
}

type setBusyRequest struct {
	Busy      bool       `json:"busy"`
	BusyUntil *time.Time `json:"busy_until"`
}

type reserveRequest struct {
	From  time.Time  `json:"from"`
	Until *time.Time `json:"until"`
}

type objectResponse struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	ImagePath    string            `json:"image_path"`
	Busy         bool              `json:"busy"`
	BusyUntil    *time.Time        `json:"busy_until"`
	ReservedFrom *time.Time        `json:"reserved_from"`
	Category     *categoryResponse `json:"category"`
	Tags         []string          `json:"tags"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type objectEnvelope struct {
	Object objectResponse `json:"object"`
}

type objectsEnvelope struct {
	Objects []objectResponse `json:"objects"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

func (s *Server) createObject(w http.ResponseWriter, r *http.Request) {
	var input usecase.CreateObjectInput
	if isMultipartForm(r) {
		parsed, cleanup, err := parseCreateMultipart(w, r)
		if err != nil {
			writeError(w, err)
			return
		}
		defer cleanup()
		input = parsed
	} else {
		var request createObjectRequest
		if !decodeJSON(w, r, &request) {
			return
		}
		input = usecase.CreateObjectInput{
			Name:        request.Name,
			Description: request.Description,
			CategoryID:  request.CategoryID,
			Tags:        request.Tags,
		}
	}
	object, err := s.objects.Create(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, objectEnvelope{Object: toObjectResponse(object)})
}

func (s *Server) listObjects(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 20)
	if err != nil {
		writeError(w, err)
		return
	}
	if limit == 0 {
		limit = 20
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		writeError(w, err)
		return
	}
	objects, total, err := s.objects.List(r.Context(), usecase.ListObjectsInput{
		Query:    r.URL.Query().Get("q"),
		Category: r.URL.Query().Get("category"),
		Tag:      r.URL.Query().Get("tag"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response := make([]objectResponse, 0, len(objects))
	for _, object := range objects {
		response = append(response, toObjectResponse(object))
	}
	writeJSON(w, http.StatusOK, objectsEnvelope{
		Objects: response,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	})
}

func (s *Server) getObject(w http.ResponseWriter, r *http.Request) {
	object, err := s.objects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objectEnvelope{Object: toObjectResponse(object)})
}

func (s *Server) updateObject(w http.ResponseWriter, r *http.Request) {
	var input usecase.UpdateObjectInput
	if isMultipartForm(r) {
		parsed, cleanup, err := parseUpdateMultipart(w, r)
		if err != nil {
			writeError(w, err)
			return
		}
		defer cleanup()
		input = parsed
	} else {
		var request updateObjectRequest
		if !decodeJSON(w, r, &request) {
			return
		}
		input = usecase.UpdateObjectInput{
			Name:        request.Name,
			Description: request.Description,
			CategoryID:  request.CategoryID,
			Tags:        request.Tags,
		}
		if request.RemovePhoto != nil {
			input.RemovePhoto = *request.RemovePhoto
		}
	}
	object, err := s.objects.Update(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objectEnvelope{Object: toObjectResponse(object)})
}

func (s *Server) setObjectBusy(w http.ResponseWriter, r *http.Request) {
	var request setBusyRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	object, err := s.objects.SetBusy(r.Context(), r.PathValue("id"), usecase.SetObjectBusyInput{
		Busy:      request.Busy,
		BusyUntil: request.BusyUntil,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objectEnvelope{Object: toObjectResponse(object)})
}

func (s *Server) reserveObject(w http.ResponseWriter, r *http.Request) {
	var request reserveRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	object, err := s.objects.Reserve(r.Context(), r.PathValue("id"), usecase.ReserveObjectInput{
		From:  request.From,
		Until: request.Until,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objectEnvelope{Object: toObjectResponse(object)})
}

func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request) {
	if err := s.objects.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toObjectResponse(object domain.Object) objectResponse {
	response := objectResponse{
		ID:           object.ID,
		Name:         object.Name,
		Description:  object.Description,
		ImagePath:    imageURL(object.ImagePath),
		Busy:         object.IsBusyAt(time.Now().UTC()),
		BusyUntil:    object.BusyUntil,
		ReservedFrom: object.ReservedFrom,
		Tags:         object.Tags,
		CreatedAt:    object.CreatedAt,
		UpdatedAt:    object.UpdatedAt,
	}
	if response.Tags == nil {
		response.Tags = []string{}
	}
	if object.Category != nil {
		category := toCategoryResponse(*object.Category)
		response.Category = &category
	}
	return response
}

func imageURL(fileName string) string {
	if fileName == "" {
		return ""
	}
	// Imported objects can carry an absolute URL or a root-relative public path.
	if strings.HasPrefix(fileName, "/") || strings.HasPrefix(fileName, "http://") || strings.HasPrefix(fileName, "https://") {
		return fileName
	}
	return imageURLPrefix + fileName
}

func queryInt(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", domain.ErrInvalid, name)
	}
	return value, nil
}
