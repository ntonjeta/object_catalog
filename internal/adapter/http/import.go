package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"biblio-oggetto/internal/adapter/spreadsheet"
	"biblio-oggetto/internal/domain"
)

const importMultipartOverhead = 1 << 20

type importFailureResponse struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type importResponse struct {
	Created  int                     `json:"created"`
	Updated  int                     `json:"updated"`
	Failed   int                     `json:"failed"`
	Failures []importFailureResponse `json:"failures"`
}

func (s *Server) importObjects(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, spreadsheet.MaxFileBytes+importMultipartOverhead)
	if err := r.ParseMultipartForm(spreadsheet.MaxFileBytes); err != nil {
		writeError(w, fmt.Errorf("%w: import must be multipart/form-data with a file field", domain.ErrInvalid))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if errors.Is(err, http.ErrMissingFile) {
		writeError(w, fmt.Errorf("%w: import file is required", domain.ErrInvalid))
		return
	}
	if err != nil {
		writeError(w, fmt.Errorf("%w: invalid import upload", domain.ErrInvalid))
		return
	}
	defer file.Close()

	rows, err := spreadsheet.Parse(header.Filename, file)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := s.objects.Import(r.Context(), rows)
	if err != nil {
		writeError(w, err)
		return
	}

	failures := make([]importFailureResponse, 0, len(result.Failures))
	for _, failure := range result.Failures {
		failures = append(failures, importFailureResponse{
			Row:     failure.Row,
			Message: failure.Message,
		})
	}
	writeJSON(w, http.StatusOK, importResponse{
		Created:  result.Created,
		Updated:  result.Updated,
		Failed:   len(failures),
		Failures: failures,
	})
}
