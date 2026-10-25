package usecase

import (
	"context"
	"io"
)

// MaxPhotoBytes is the maximum accepted size for an uploaded object photo.
const MaxPhotoBytes = 5 << 20

type PhotoUpload struct {
	Reader      io.Reader
	Filename    string
	ContentType string
}

// PhotoStore persists object photos and returns the stored file name.
// Delete must be safe to call with a file that no longer exists.
type PhotoStore interface {
	Save(ctx context.Context, upload PhotoUpload) (fileName string, err error)
	Delete(ctx context.Context, fileName string) error
}
