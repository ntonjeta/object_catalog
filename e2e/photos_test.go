package e2e

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"biblio-oggetto/internal/usecase"
)

func TestObjectPhotoCreateServeReplaceRemove(t *testing.T) {
	environment := newEnv(t)
	environment.login()

	first := testPNG(t, 2, 2, color.RGBA{R: 220, A: 255})
	response, data := environment.doMultipart(environment.admin, http.MethodPost, "/api/objects", map[string]string{
		"name":        "Dune",
		"description": "A desert planet",
		"tags":        "scifi, classic",
	}, first, "cover.png")
	assertStatus(t, response, http.StatusCreated, data)
	object := decode[objectEnvelope](t, data).Object
	if !strings.HasPrefix(object.ImagePath, "/static/uploads/") {
		t.Fatalf("image path = %q, want /static/uploads/ prefix", object.ImagePath)
	}
	if !equalStrings(object.Tags, []string{"scifi", "classic"}) {
		t.Fatalf("tags = %v", object.Tags)
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, object.ImagePath, nil)
	assertStatus(t, response, http.StatusOK, data)
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "image/png") {
		t.Fatalf("photo content type = %q, want image/png", contentType)
	}
	if !bytes.Equal(data, first) {
		t.Fatal("served photo does not match uploaded bytes")
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusOK, data)
	public := decode[objectEnvelope](t, data).Object
	if public.ImagePath != object.ImagePath {
		t.Fatalf("public image path = %q, want %q", public.ImagePath, object.ImagePath)
	}
	if len(public.Images) != 1 || public.Images[0].URL != public.ImagePath || public.Images[0].Position != 0 {
		t.Fatalf("public single-photo list = %+v, want row 0 seeded from the photo", public.Images)
	}

	second := testPNG(t, 3, 3, color.RGBA{B: 220, A: 255})
	response, data = environment.doMultipart(environment.admin, http.MethodPatch, "/api/objects/"+object.ID, nil, second, "replacement.png")
	assertStatus(t, response, http.StatusOK, data)
	replaced := decode[objectEnvelope](t, data).Object
	if replaced.ImagePath == object.ImagePath {
		t.Fatal("image path was not replaced")
	}
	if len(replaced.Images) != 1 || replaced.Images[0].URL != replaced.ImagePath || replaced.Images[0].ID != public.Images[0].ID {
		t.Fatalf("replaced single-photo list = %+v, want row 0 overwritten with its id kept", replaced.Images)
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, object.ImagePath, nil)
	assertStatus(t, response, http.StatusNotFound, data)
	response, data = environment.do(environment.anonymous, http.MethodGet, replaced.ImagePath, nil)
	assertStatus(t, response, http.StatusOK, data)

	response, data = environment.doMultipart(environment.admin, http.MethodPatch, "/api/objects/"+object.ID, map[string]string{
		"remove_photo": "true",
	}, nil, "")
	assertStatus(t, response, http.StatusOK, data)
	removed := decode[objectEnvelope](t, data).Object
	if removed.ImagePath != "" {
		t.Fatalf("image path = %q, want empty", removed.ImagePath)
	}
	if len(removed.Images) != 0 {
		t.Fatalf("removed single-photo list = %+v, want empty", removed.Images)
	}
	response, data = environment.do(environment.anonymous, http.MethodGet, replaced.ImagePath, nil)
	assertStatus(t, response, http.StatusNotFound, data)

	response, data = environment.doMultipart(environment.admin, http.MethodPatch, "/api/objects/"+object.ID, nil, second, "again.png")
	assertStatus(t, response, http.StatusOK, data)
	withPhoto := decode[objectEnvelope](t, data).Object
	response, data = environment.do(environment.admin, http.MethodDelete, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusNoContent, data)
	response, data = environment.do(environment.anonymous, http.MethodGet, withPhoto.ImagePath, nil)
	assertStatus(t, response, http.StatusNotFound, data)
}

func TestObjectPhotoValidation(t *testing.T) {
	environment := newEnv(t)
	photo := testPNG(t, 1, 1, color.White)

	response, data := environment.doMultipart(environment.anonymous, http.MethodPost, "/api/objects", map[string]string{
		"name": "Anonymous",
	}, photo, "photo.png")
	assertStatus(t, response, http.StatusUnauthorized, data)

	environment.login()
	response, data = environment.doMultipart(environment.admin, http.MethodPost, "/api/objects", map[string]string{
		"name": "Not an image",
	}, []byte("plain text"), "photo.png")
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	response, data = environment.doMultipart(environment.admin, http.MethodPost, "/api/objects", map[string]string{
		"name": "Too large",
	}, bytes.Repeat([]byte("a"), usecase.MaxPhotoBytes+1), "large.png")
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	response, data = environment.do(environment.admin, http.MethodPost, "/api/objects", map[string]any{
		"name": "JSON object",
	})
	assertStatus(t, response, http.StatusCreated, data)
	if object := decode[objectEnvelope](t, data).Object; object.ImagePath != "" {
		t.Fatalf("JSON image path = %q, want empty", object.ImagePath)
	}
}

func (e *env) doMultipart(client *http.Client, method, path string, fields map[string]string, photo []byte, filename string) (*http.Response, []byte) {
	e.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			e.t.Fatalf("write multipart field: %v", err)
		}
	}
	if photo != nil {
		part, err := writer.CreateFormFile("photo", filename)
		if err != nil {
			e.t.Fatalf("create photo part: %v", err)
		}
		if _, err := part.Write(photo); err != nil {
			e.t.Fatalf("write photo part: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		e.t.Fatalf("close multipart body: %v", err)
	}

	request, err := http.NewRequest(method, e.server.URL+path, &body)
	if err != nil {
		e.t.Fatalf("new multipart request: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		e.t.Fatalf("read multipart response: %v", err)
	}
	return response, data
}

func testPNG(t *testing.T, width, height int, fill color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(fill), image.Point{}, draw.Src)
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}
