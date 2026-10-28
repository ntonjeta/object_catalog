package e2e

import (
	"bytes"
	"image/color"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type imagePart struct {
	filename string
	data     []byte
}

func (e *env) postImages(client *http.Client, path string, parts ...imagePart) (*http.Response, []byte) {
	e.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		file, err := writer.CreateFormFile("images", part.filename)
		if err != nil {
			e.t.Fatalf("create images part: %v", err)
		}
		if _, err := file.Write(part.data); err != nil {
			e.t.Fatalf("write images part: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		e.t.Fatalf("close images body: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, e.server.URL+path, &body)
	if err != nil {
		e.t.Fatalf("new images request: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := client.Do(request)
	if err != nil {
		e.t.Fatalf("POST %s: %v", path, err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		e.t.Fatalf("read images response: %v", err)
	}
	return response, data
}

func imageFileName(t *testing.T, url string) string {
	t.Helper()
	const prefix = "/static/uploads/"
	if !strings.HasPrefix(url, prefix) {
		t.Fatalf("image url = %q, want %s prefix", url, prefix)
	}
	return strings.TrimPrefix(url, prefix)
}

func imageIDs(images []objectImageDTO) []string {
	ids := make([]string, len(images))
	for i, image := range images {
		ids[i] = image.ID
	}
	return ids
}

func TestObjectImagesUploadServeReorderRemove(t *testing.T) {
	environment := newEnv(t)
	environment.login()
	object := environment.createObject(map[string]any{"name": "Gallery"})
	if len(object.Images) != 0 {
		t.Fatalf("fresh images = %+v, want empty", object.Images)
	}

	red := testPNG(t, 2, 2, color.RGBA{R: 220, A: 255})
	blue := testPNG(t, 3, 3, color.RGBA{B: 220, A: 255})
	response, data := environment.postImages(environment.admin, "/api/objects/"+object.ID+"/images",
		imagePart{filename: "a.png", data: red},
		imagePart{filename: "b.png", data: blue},
	)
	assertStatus(t, response, http.StatusOK, data)
	uploaded := decode[objectEnvelope](t, data).Object
	if len(uploaded.Images) != 2 {
		t.Fatalf("uploaded images = %+v, want two", uploaded.Images)
	}
	for i, image := range uploaded.Images {
		if image.ID == "" || image.Position != i {
			t.Fatalf("image %d = %+v, want id and position %d", i, image, i)
		}
	}
	if uploaded.Images[0].ID == uploaded.Images[1].ID {
		t.Fatalf("image ids repeat: %+v", uploaded.Images)
	}
	if uploaded.ImagePath != uploaded.Images[0].URL {
		t.Fatalf("cover = %q, want images[0] url %q", uploaded.ImagePath, uploaded.Images[0].URL)
	}

	sources := [][]byte{red, blue}
	for i, image := range uploaded.Images {
		response, data = environment.do(environment.anonymous, http.MethodGet, image.URL, nil)
		assertStatus(t, response, http.StatusOK, data)
		if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "image/png") {
			t.Fatalf("image %d content type = %q, want image/png", i, contentType)
		}
		if !bytes.Equal(data, sources[i]) {
			t.Fatalf("served image %d does not match uploaded bytes", i)
		}
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusOK, data)
	if single := decode[objectEnvelope](t, data).Object; len(single.Images) != 2 || single.Images[0].ID != uploaded.Images[0].ID {
		t.Fatalf("public single images = %+v, want two rows", single.Images)
	}
	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects?limit=100", nil)
	assertStatus(t, response, http.StatusOK, data)
	listed := findObject(t, decode[objectsEnvelope](t, data).Objects, "Gallery")
	if len(listed.Images) != 2 || listed.Images[1].ID != uploaded.Images[1].ID {
		t.Fatalf("public list images = %+v, want two rows", listed.Images)
	}

	firstID, secondID := uploaded.Images[0].ID, uploaded.Images[1].ID
	secondURL := uploaded.Images[1].URL
	response, data = environment.do(environment.admin, http.MethodPatch, "/api/objects/"+object.ID+"/images/order",
		map[string]any{"image_ids": []string{secondID, firstID}})
	assertStatus(t, response, http.StatusOK, data)
	reordered := decode[objectEnvelope](t, data).Object
	if len(reordered.Images) != 2 || reordered.Images[0].ID != secondID || reordered.Images[1].ID != firstID {
		t.Fatalf("reordered images = %+v, want [%s %s]", reordered.Images, secondID, firstID)
	}
	for i, image := range reordered.Images {
		if image.Position != i {
			t.Fatalf("reordered position = %+v, want %d", image, i)
		}
	}
	if reordered.ImagePath != secondURL {
		t.Fatalf("cover after reorder = %q, want %q", reordered.ImagePath, secondURL)
	}

	response, data = environment.do(environment.admin, http.MethodDelete, "/api/objects/"+object.ID+"/images/"+secondID, nil)
	assertStatus(t, response, http.StatusOK, data)
	reduced := decode[objectEnvelope](t, data).Object
	if len(reduced.Images) != 1 || reduced.Images[0].ID != firstID {
		t.Fatalf("images after remove = %+v, want only %s", reduced.Images, firstID)
	}
	if reduced.ImagePath != reduced.Images[0].URL {
		t.Fatalf("cover after remove = %q, want new row 0 url %q", reduced.ImagePath, reduced.Images[0].URL)
	}
	response, data = environment.do(environment.anonymous, http.MethodGet, secondURL, nil)
	assertStatus(t, response, http.StatusNotFound, data)
}

func TestObjectImagesCapAndValidation(t *testing.T) {
	environment := newEnv(t)
	environment.login()

	response, data := environment.do(environment.admin, http.MethodPost, "/api/objects", map[string]any{
		"name": "Cap",
	})
	assertStatus(t, response, http.StatusCreated, data)
	if !bytes.Contains(data, []byte(`"images":[]`)) {
		t.Fatalf("empty image list must marshal as [], body: %s", data)
	}
	object := decode[objectEnvelope](t, data).Object
	path := "/api/objects/" + object.ID + "/images"

	ten := make([]imagePart, 0, 10)
	for i := 0; i < 10; i++ {
		ten = append(ten, imagePart{filename: "img.png", data: testPNG(t, 1, 1, color.White)})
	}
	response, data = environment.postImages(environment.admin, path, append(ten, imagePart{
		filename: "over.png", data: testPNG(t, 1, 1, color.White),
	})...)
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	response, data = environment.postImages(environment.admin, path, ten...)
	assertStatus(t, response, http.StatusOK, data)
	full := decode[objectEnvelope](t, data).Object
	if len(full.Images) != 10 {
		t.Fatalf("images after full upload = %+v, want ten", full.Images)
	}

	response, data = environment.postImages(environment.admin, path, imagePart{
		filename: "over.png", data: testPNG(t, 1, 1, color.White),
	})
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	ids := imageIDs(full.Images)
	response, data = environment.do(environment.admin, http.MethodPatch, path+"/order", map[string]any{
		"image_ids": ids[:len(ids)-1],
	})
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	swapped := append([]string{}, ids...)
	swapped[len(swapped)-1] = "nonexistent-image-id"
	response, data = environment.do(environment.admin, http.MethodPatch, path+"/order", map[string]any{
		"image_ids": swapped,
	})
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	response, data = environment.do(environment.admin, http.MethodDelete, path+"/nonexistent-image-id", nil)
	assertStatus(t, response, http.StatusNotFound, data)
	assertErrorCode(t, data, "not_found")

	response, data = environment.do(environment.admin, http.MethodDelete, path+"/order", nil)
	assertStatus(t, response, http.StatusNotFound, data)
	assertErrorCode(t, data, "not_found")

	response, data = environment.postImages(environment.admin, path)
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	response, data = environment.do(environment.anonymous, http.MethodPatch, path+"/order", map[string]any{
		"image_ids": ids,
	})
	assertStatus(t, response, http.StatusUnauthorized, data)
	assertErrorCode(t, data, "unauthorized")

	response, data = environment.postImages(environment.anonymous, path, imagePart{
		filename: "anon.png", data: testPNG(t, 1, 1, color.White),
	})
	assertStatus(t, response, http.StatusUnauthorized, data)
	assertErrorCode(t, data, "unauthorized")

	response, data = environment.postImages(environment.admin, "/api/objects/nonexistent-id/images", imagePart{
		filename: "orphan.png", data: testPNG(t, 1, 1, color.White),
	})
	assertStatus(t, response, http.StatusNotFound, data)
	assertErrorCode(t, data, "not_found")
}

func TestObjectDeleteRemovesAllImageFiles(t *testing.T) {
	environment := newEnv(t)
	environment.login()
	object := environment.createObject(map[string]any{"name": "Doomed"})

	response, data := environment.postImages(environment.admin, "/api/objects/"+object.ID+"/images",
		imagePart{filename: "one.png", data: testPNG(t, 1, 1, color.White)},
		imagePart{filename: "two.png", data: testPNG(t, 2, 2, color.Black)},
	)
	assertStatus(t, response, http.StatusOK, data)
	uploaded := decode[objectEnvelope](t, data).Object
	if len(uploaded.Images) != 2 {
		t.Fatalf("images = %+v, want two", uploaded.Images)
	}

	uploadDir := filepath.Join(environment.uploadRoot, "uploads")
	names := make([]string, 0, len(uploaded.Images))
	for _, image := range uploaded.Images {
		name := imageFileName(t, image.URL)
		if _, err := os.Stat(filepath.Join(uploadDir, name)); err != nil {
			t.Fatalf("uploaded file %s missing before delete: %v", name, err)
		}
		names = append(names, name)
	}

	response, data = environment.do(environment.admin, http.MethodDelete, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusNoContent, data)
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(uploadDir, name)); !os.IsNotExist(err) {
			t.Fatalf("file %s survived object delete (stat err = %v)", name, err)
		}
	}
}
