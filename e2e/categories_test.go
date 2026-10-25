package e2e

import (
	"net/http"
	"testing"
)

func TestCategoryCRUD(t *testing.T) {
	environment := newEnv(t)
	environment.login()

	response, data := environment.do(environment.anonymous, http.MethodGet, "/api/categories", nil)
	assertStatus(t, response, http.StatusOK, data)
	if categories := decode[categoriesEnvelope](t, data).Categories; len(categories) != 0 {
		t.Fatalf("categories = %d, want 0", len(categories))
	}

	response, data = environment.do(environment.admin, http.MethodPost, "/api/categories", map[string]string{
		"name": "  Science   Fiction ",
	})
	assertStatus(t, response, http.StatusCreated, data)
	created := decode[categoryEnvelope](t, data).Category
	if created.Name != "Science Fiction" {
		t.Fatalf("name = %q, want normalized %q", created.Name, "Science Fiction")
	}

	response, data = environment.do(environment.admin, http.MethodPost, "/api/categories", map[string]string{
		"name": "science fiction",
	})
	assertStatus(t, response, http.StatusConflict, data)
	assertErrorCode(t, data, "conflict")

	response, data = environment.do(environment.admin, http.MethodPost, "/api/categories", map[string]string{
		"name": "   ",
	})
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	response, data = environment.doRaw(environment.admin, http.MethodPost, "/api/categories", []byte("{"))
	assertStatus(t, response, http.StatusBadRequest, data)

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/categories", nil)
	assertStatus(t, response, http.StatusOK, data)
	if categories := decode[categoriesEnvelope](t, data).Categories; len(categories) != 1 {
		t.Fatalf("categories = %d, want 1", len(categories))
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/categories/"+created.ID, nil)
	assertStatus(t, response, http.StatusOK, data)
	if fetched := decode[categoryEnvelope](t, data).Category; fetched.ID != created.ID {
		t.Fatalf("category id = %q, want %q", fetched.ID, created.ID)
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/categories/missing", nil)
	assertStatus(t, response, http.StatusNotFound, data)

	response, data = environment.do(environment.admin, http.MethodPatch, "/api/categories/"+created.ID, map[string]string{
		"name": "Speculative Fiction",
	})
	assertStatus(t, response, http.StatusOK, data)
	if updated := decode[categoryEnvelope](t, data).Category; updated.Name != "Speculative Fiction" {
		t.Fatalf("name = %q, want %q", updated.Name, "Speculative Fiction")
	}

	response, data = environment.do(environment.admin, http.MethodPatch, "/api/categories/missing", map[string]string{
		"name": "Missing",
	})
	assertStatus(t, response, http.StatusNotFound, data)

	response, data = environment.do(environment.admin, http.MethodDelete, "/api/categories/missing", nil)
	assertStatus(t, response, http.StatusNotFound, data)

	response, data = environment.do(environment.admin, http.MethodDelete, "/api/categories/"+created.ID, nil)
	assertStatus(t, response, http.StatusNoContent, data)

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/categories", nil)
	assertStatus(t, response, http.StatusOK, data)
	if categories := decode[categoriesEnvelope](t, data).Categories; len(categories) != 0 {
		t.Fatalf("categories = %d, want 0", len(categories))
	}
}
