package httpapi

import (
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

const multipartOverhead = 1 << 20

// maxObjectImages mirrors the unexported domain cap so a single POST cannot
// stream more files than an object may hold; the use case re-validates the
// prospective total.
const maxObjectImages = 10

func isMultipartForm(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "multipart/form-data"
}

func parseCreateMultipart(w http.ResponseWriter, r *http.Request) (usecase.CreateObjectInput, func(), error) {
	form, cleanup, err := parseMultipart(w, r)
	if err != nil {
		return usecase.CreateObjectInput{}, nil, err
	}
	input := usecase.CreateObjectInput{
		Name:        firstValue(form.Value["name"]),
		Description: firstValue(form.Value["description"]),
		CategoryID:  firstValue(form.Value["category_id"]),
		Tags:        multipartTags(form.Value["tags"]),
	}
	input.Photo, cleanup, err = multipartPhoto(r, cleanup)
	if err != nil {
		return usecase.CreateObjectInput{}, nil, err
	}
	return input, cleanup, nil
}

func parseUpdateMultipart(w http.ResponseWriter, r *http.Request) (usecase.UpdateObjectInput, func(), error) {
	form, cleanup, err := parseMultipart(w, r)
	if err != nil {
		return usecase.UpdateObjectInput{}, nil, err
	}
	input := usecase.UpdateObjectInput{}
	if values, ok := form.Value["name"]; ok {
		value := firstValue(values)
		input.Name = &value
	}
	if values, ok := form.Value["description"]; ok {
		value := firstValue(values)
		input.Description = &value
	}
	if values, ok := form.Value["category_id"]; ok {
		value := firstValue(values)
		input.CategoryID = &value
	}
	if values, ok := form.Value["tags"]; ok {
		value := multipartTags(values)
		input.Tags = &value
	}
	if values, ok := form.Value["remove_photo"]; ok {
		remove, err := strconv.ParseBool(strings.TrimSpace(firstValue(values)))
		if err != nil {
			cleanup()
			return usecase.UpdateObjectInput{}, nil, fmt.Errorf("%w: remove_photo must be true or false", domain.ErrInvalid)
		}
		input.RemovePhoto = remove
	}
	input.Photo, cleanup, err = multipartPhoto(r, cleanup)
	if err != nil {
		return usecase.UpdateObjectInput{}, nil, err
	}
	return input, cleanup, nil
}

func parseMultipart(w http.ResponseWriter, r *http.Request) (*multipart.Form, func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxPhotoBytes+multipartOverhead)
	if err := r.ParseMultipartForm(usecase.MaxPhotoBytes); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid multipart form", domain.ErrInvalid)
	}
	cleanup := func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}
	return r.MultipartForm, cleanup, nil
}

func multipartPhoto(r *http.Request, cleanup func()) (*usecase.PhotoUpload, func(), error) {
	file, header, err := r.FormFile("photo")
	if errors.Is(err, http.ErrMissingFile) {
		return nil, cleanup, nil
	}
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("%w: invalid photo upload", domain.ErrInvalid)
	}
	upload := &usecase.PhotoUpload{
		Reader:      file,
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
	}
	wrapped := func() {
		_ = file.Close()
		cleanup()
	}
	return upload, wrapped, nil
}

func parseAddImagesMultipart(w http.ResponseWriter, r *http.Request) ([]usecase.PhotoUpload, func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxObjectImages)*(usecase.MaxPhotoBytes+multipartOverhead))
	if err := r.ParseMultipartForm(usecase.MaxPhotoBytes); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid multipart form", domain.ErrInvalid)
	}
	cleanup := func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}
	return multipartPhotos(r, cleanup)
}

// multipartPhotos reads the repeatable `images` file parts. Per-file size and
// content sniffing stay in filestore.Save; this only bounds the request and
// opens the files for the use case to stream.
func multipartPhotos(r *http.Request, cleanup func()) ([]usecase.PhotoUpload, func(), error) {
	headers := r.MultipartForm.File["images"]
	if len(headers) == 0 {
		cleanup()
		return nil, nil, fmt.Errorf("%w: at least one image is required", domain.ErrInvalid)
	}
	if len(headers) > maxObjectImages {
		cleanup()
		return nil, nil, fmt.Errorf("%w: at most %d images per upload", domain.ErrInvalid, maxObjectImages)
	}
	files := make([]multipart.File, 0, len(headers))
	uploads := make([]usecase.PhotoUpload, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			for _, open := range files {
				_ = open.Close()
			}
			cleanup()
			return nil, nil, fmt.Errorf("%w: invalid image upload", domain.ErrInvalid)
		}
		files = append(files, file)
		uploads = append(uploads, usecase.PhotoUpload{
			Reader:      file,
			Filename:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
		})
	}
	wrapped := func() {
		for _, file := range files {
			_ = file.Close()
		}
		cleanup()
	}
	return uploads, wrapped, nil
}

func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func multipartTags(values []string) []string {
	tags := make([]string, 0)
	for _, value := range values {
		for _, tag := range strings.Split(value, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}
