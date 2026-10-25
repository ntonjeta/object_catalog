package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

func TestStoreSavesAndDeletesPhoto(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	data := photoPNG(t)
	name, err := store.Save(context.Background(), usecase.PhotoUpload{
		Reader:      bytes.NewReader(data),
		Filename:    "../../escape.png",
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || name != filepath.Base(name) {
		t.Fatalf("unsafe generated name %q", name)
	}
	stored, err := os.ReadFile(filepath.Join(store.Dir(), name))
	if err != nil {
		t.Fatalf("read stored photo: %v", err)
	}
	if !bytes.Equal(stored, data) {
		t.Fatal("stored photo does not match input")
	}

	if err := store.Delete(context.Background(), name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("photo still exists or wrong error: %v", err)
	}
	if err := store.Delete(context.Background(), name); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestStoreAcceptsWebP(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(webpFixture)
	if err != nil {
		t.Fatalf("decode webp fixture: %v", err)
	}
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	publicPath, err := store.Save(context.Background(), usecase.PhotoUpload{Reader: bytes.NewReader(data)})
	if err != nil {
		t.Fatalf("Save webp: %v", err)
	}
	if !strings.HasSuffix(publicPath, ".webp") {
		t.Fatalf("public path = %q, want .webp suffix", publicPath)
	}
}

func TestStoreRejectsInvalidPhotos(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "text", data: []byte("not an image")},
		{name: "html", data: []byte("<html><script>alert(1)</script></html>")},
		{name: "oversized", data: bytes.Repeat([]byte("a"), usecase.MaxPhotoBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.Save(context.Background(), usecase.PhotoUpload{Reader: bytes.NewReader(test.data)})
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Save error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestStoreRejectsUnsafeDeletePath(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	outside := filepath.Join(filepath.Dir(store.Dir()), "outside.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	paths := []string{
		"/static/uploads/../outside.txt",
		"/static/uploads/",
		"/other/outside.txt",
	}
	for _, path := range paths {
		if err := store.Delete(context.Background(), path); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("Delete(%q) error = %v, want ErrInvalid", path, err)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was removed: %v", err)
	}
}

func photoPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 200, G: 100, A: 255}), image.Point{}, draw.Src)
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}

const webpFixture = "UklGRh4CAABXRUJQVlA4WAoAAAAgAAAAAQAAAQAASUNDUMgBAAAAAAHIAAAAAAQwAABtbnRyUkdCIFhZWiAH4AABAAEAAAAAAABhY3NwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAA9tYAAQAAAADTLQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAlkZXNjAAAA8AAAACRyWFlaAAABFAAAABRnWFlaAAABKAAAABRiWFlaAAABPAAAABR3dHB0AAABUAAAABRyVFJDAAABZAAAAChnVFJDAAABZAAAAChiVFJDAAABZAAAAChjcHJ0AAABjAAAADxtbHVjAAAAAAAAAAEAAAAMZW5VUwAAAAgAAAAcAHMAUgBHAEJYWVogAAAAAAAAb6IAADj1AAADkFhZWiAAAAAAAABimQAAt4UAABjaWFlaIAAAAAAAACSgAAAPhAAAts9YWVogAAAAAAAA9tYAAQAAAADTLXBhcmEAAAAAAAQAAAACZmYAAPKnAAANWQAAE9AAAApbAAAAAAAAAABtbHVjAAAAAAAAAAEAAAAMZW5VUwAAACAAAAAcAEcAbwBvAGcAbABlACAASQBuAGMALgAgADIAMAAxADZWUDggMAAAANABAJ0BKgIAAgABQCYloAJ0ugH4AAOwAP7y63/82BXNc+/3/9Lg/S4P0uD/0pAAAA=="
