package e2e

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"biblio-oggetto/internal/domain"
)

func TestLoginMeAndLogout(t *testing.T) {
	environment := newEnv(t)

	response, data := environment.do(environment.admin, http.MethodGet, "/api/auth/me", nil)
	assertStatus(t, response, http.StatusUnauthorized, data)
	assertErrorCode(t, data, "unauthorized")

	response, data = environment.do(environment.admin, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "wrong-password",
	})
	assertStatus(t, response, http.StatusUnauthorized, data)

	response, data = environment.do(environment.admin, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    adminEmail,
		"password": adminPassword,
	})
	assertStatus(t, response, http.StatusOK, data)
	if user := decode[userEnvelope](t, data).User; user.Email != adminEmail {
		t.Fatalf("login email = %q, want %q", user.Email, adminEmail)
	}

	response, data = environment.do(environment.admin, http.MethodGet, "/api/auth/me", nil)
	assertStatus(t, response, http.StatusOK, data)
	if user := decode[userEnvelope](t, data).User; user.Email != adminEmail {
		t.Fatalf("me email = %q, want %q", user.Email, adminEmail)
	}

	response, data = environment.do(environment.admin, http.MethodPost, "/api/auth/logout", nil)
	assertStatus(t, response, http.StatusNoContent, data)

	response, data = environment.do(environment.admin, http.MethodGet, "/api/auth/me", nil)
	assertStatus(t, response, http.StatusUnauthorized, data)
}

func TestPublicReadsAndAdminWrites(t *testing.T) {
	environment := newEnv(t)
	environment.login()
	category := environment.createCategory("Books")
	object := environment.createObject(map[string]any{
		"name":        "Dune",
		"description": "desert planet",
		"category_id": category.ID,
		"tags":        []string{"scifi"},
	})

	response, data := environment.do(environment.anonymous, http.MethodGet, "/api/categories", nil)
	assertStatus(t, response, http.StatusOK, data)
	if categories := decode[categoriesEnvelope](t, data).Categories; len(categories) != 1 {
		t.Fatalf("categories = %d, want 1", len(categories))
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects", nil)
	assertStatus(t, response, http.StatusOK, data)
	if objects := decode[objectsEnvelope](t, data).Objects; len(objects) != 1 {
		t.Fatalf("objects = %d, want 1", len(objects))
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusOK, data)

	writes := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/api/categories", map[string]string{"name": "Nope"}},
		{http.MethodPatch, "/api/categories/" + category.ID, map[string]string{"name": "Nope"}},
		{http.MethodDelete, "/api/categories/" + category.ID, nil},
		{http.MethodPost, "/api/objects", map[string]string{"name": "Nope"}},
		{http.MethodPatch, "/api/objects/" + object.ID, map[string]string{"name": "Nope"}},
		{http.MethodDelete, "/api/objects/" + object.ID, nil},
	}
	for _, write := range writes {
		response, data := environment.do(environment.anonymous, write.method, write.path, write.body)
		assertStatus(t, response, http.StatusUnauthorized, data)
		assertErrorCode(t, data, "unauthorized")
	}
}

func TestChangePassword(t *testing.T) {
	environment := newEnv(t)
	environment.login()

	response, data := environment.do(environment.admin, http.MethodPut, "/api/account/password", map[string]string{
		"current_password": "wrong-password",
		"new_password":     "newpassword123",
	})
	assertStatus(t, response, http.StatusUnauthorized, data)

	response, data = environment.do(environment.admin, http.MethodPut, "/api/account/password", map[string]string{
		"current_password": adminPassword,
		"new_password":     "short",
	})
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")

	response, data = environment.do(environment.admin, http.MethodPut, "/api/account/password", map[string]string{
		"current_password": adminPassword,
		"new_password":     "newpassword123",
	})
	assertStatus(t, response, http.StatusNoContent, data)

	response, data = environment.do(environment.admin, http.MethodGet, "/api/auth/me", nil)
	assertStatus(t, response, http.StatusOK, data)

	response, data = environment.do(environment.anonymous, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    adminEmail,
		"password": adminPassword,
	})
	assertStatus(t, response, http.StatusUnauthorized, data)

	response, data = environment.do(environment.anonymous, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "newpassword123",
	})
	assertStatus(t, response, http.StatusOK, data)
}

func TestDeleteAccount(t *testing.T) {
	environment := newEnv(t)
	environment.login()

	response, data := environment.do(environment.admin, http.MethodDelete, "/api/account", nil)
	assertStatus(t, response, http.StatusNoContent, data)

	response, data = environment.do(environment.admin, http.MethodGet, "/api/auth/me", nil)
	assertStatus(t, response, http.StatusUnauthorized, data)

	response, data = environment.do(environment.anonymous, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    adminEmail,
		"password": adminPassword,
	})
	assertStatus(t, response, http.StatusUnauthorized, data)
}

func TestCreateAdminValidation(t *testing.T) {
	environment := newEnv(t)
	ctx := context.Background()

	if _, err := environment.app.CreateAdmin(ctx, "not-an-email", "password123"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid email error = %v, want ErrInvalid", err)
	}
	if _, err := environment.app.CreateAdmin(ctx, "other@example.com", "short"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("short password error = %v, want ErrInvalid", err)
	}
	if _, err := environment.app.CreateAdmin(ctx, adminEmail, adminPassword); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate admin error = %v, want ErrConflict", err)
	}
}
