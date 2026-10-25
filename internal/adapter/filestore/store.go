package filestore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

const (
	uploadDirectory = "uploads"
	urlPrefix       = "/static/uploads/"
)

type Store struct {
	dir string
}

// New creates the upload directory under root and returns a filesystem-backed
// photo store.
func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("photo root directory is required")
	}
	dir, err := filepath.Abs(filepath.Join(root, uploadDirectory))
	if err != nil {
		return nil, fmt.Errorf("resolve photo directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create photo directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the absolute directory containing uploaded photos.
func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) Save(ctx context.Context, upload usecase.PhotoUpload) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(upload.Reader, usecase.MaxPhotoBytes+1))
	if err != nil {
		return "", fmt.Errorf("read photo: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%w: photo is empty", domain.ErrInvalid)
	}
	if len(data) > usecase.MaxPhotoBytes {
		return "", fmt.Errorf("%w: photo must be at most %d bytes", domain.ErrInvalid, usecase.MaxPhotoBytes)
	}
	extension, err := imageExtension(data)
	if err != nil {
		return "", err
	}

	id, err := domain.NewID()
	if err != nil {
		return "", fmt.Errorf("generate photo name: %w", err)
	}
	name := id + extension
	path := filepath.Join(s.dir, name)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("create photo: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(path)
		return "", fmt.Errorf("write photo: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("close photo: %w", err)
	}
	return name, nil
}

func (s *Store) Delete(ctx context.Context, fileName string) error {
	name, err := s.fileName(fileName)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete photo: %w", err)
	}
	return nil
}

func (s *Store) fileName(fileName string) (string, error) {
	name := strings.TrimPrefix(fileName, urlPrefix)
	if name == "" || strings.Contains(name, "/") || name != filepath.Base(name) || !fs.ValidPath(name) {
		return "", fmt.Errorf("%w: invalid photo file name", domain.ErrInvalid)
	}
	return name, nil
}

func imageExtension(data []byte) (string, error) {
	contentType := http.DetectContentType(data)
	expectedFormat := map[string]string{
		"image/jpeg": "jpeg",
		"image/png":  "png",
		"image/webp": "webp",
	}
	expected, ok := expectedFormat[contentType]
	if !ok {
		return "", fmt.Errorf("%w: photo must be JPEG, PNG, or WebP", domain.ErrInvalid)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != expected || config.Width <= 0 || config.Height <= 0 {
		return "", fmt.Errorf("%w: photo is not a valid %s image", domain.ErrInvalid, expected)
	}
	switch expected {
	case "jpeg":
		return ".jpg", nil
	case "png":
		return ".png", nil
	default:
		return ".webp", nil
	}
}
