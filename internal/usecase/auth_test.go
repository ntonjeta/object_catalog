package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"biblio-oggetto/internal/domain"
)

func newTestAuth() (*Auth, *fakeUsers, *fakeSessions) {
	users := newFakeUsers()
	sessions := newFakeSessions()
	return NewAuth(users, sessions, time.Hour), users, sessions
}

func TestAuthCreateAdminAndLogin(t *testing.T) {
	ctx := context.Background()
	auth, _, sessions := newTestAuth()

	user, err := auth.CreateAdmin(ctx, "  ADMIN@Example.com ", "password123")
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if user.Email != "admin@example.com" {
		t.Fatalf("email = %q, want admin@example.com", user.Email)
	}
	if user.PasswordHash == "password123" {
		t.Fatal("password was not hashed")
	}

	if _, err := auth.CreateAdmin(ctx, "admin@example.com", "password123"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate error = %v, want ErrConflict", err)
	}
	if _, err := auth.CreateAdmin(ctx, "bad-email", "password123"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("bad email error = %v, want ErrInvalid", err)
	}
	if _, err := auth.CreateAdmin(ctx, "other@example.com", "short"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("short password error = %v, want ErrInvalid", err)
	}

	if _, _, _, err := auth.Login(ctx, "admin@example.com", "wrong-password"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong password error = %v, want ErrUnauthorized", err)
	}
	if _, _, _, err := auth.Login(ctx, "missing@example.com", "password123"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("missing user error = %v, want ErrUnauthorized", err)
	}

	loggedIn, token, expiresAt, err := auth.Login(ctx, "ADMIN@example.com", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if loggedIn.ID != user.ID {
		t.Fatalf("login user = %q, want %q", loggedIn.ID, user.ID)
	}
	if token == "" || !expiresAt.After(time.Now()) {
		t.Fatalf("session token = %q, expires = %v", token, expiresAt)
	}
	if _, ok := sessions.sessions[hashToken(token)]; !ok {
		t.Fatal("session was not persisted by hash")
	}

	authenticated, err := auth.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if authenticated.ID != user.ID {
		t.Fatalf("authenticated user = %q, want %q", authenticated.ID, user.ID)
	}
	if _, err := auth.Authenticate(ctx, "wrong-token"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("bad token error = %v, want ErrUnauthorized", err)
	}
	if _, err := auth.Authenticate(ctx, ""); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("empty token error = %v, want ErrUnauthorized", err)
	}

	if err := auth.Logout(ctx, token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("logged out token error = %v, want ErrUnauthorized", err)
	}
	if err := auth.Logout(ctx, token); err != nil {
		t.Fatalf("second Logout: %v", err)
	}
}

func TestAuthExpiredSession(t *testing.T) {
	ctx := context.Background()
	auth, _, sessions := newTestAuth()
	if _, err := auth.CreateAdmin(ctx, "admin@example.com", "password123"); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	_, token, _, err := auth.Login(ctx, "admin@example.com", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	hash := hashToken(token)
	session := sessions.sessions[hash]
	session.expiresAt = time.Now().Add(-time.Minute)
	sessions.sessions[hash] = session

	if _, err := auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired token error = %v, want ErrUnauthorized", err)
	}
	if _, ok := sessions.sessions[hash]; ok {
		t.Fatal("expired session was not deleted")
	}
}

func TestAuthChangePasswordAndDeleteAccount(t *testing.T) {
	ctx := context.Background()
	auth, _, sessions := newTestAuth()
	user, err := auth.CreateAdmin(ctx, "admin@example.com", "password123")
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	_, oldToken, _, err := auth.Login(ctx, "admin@example.com", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, _, err := auth.ChangePassword(ctx, user.ID, "wrong-password", "newpassword123"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong current password error = %v, want ErrUnauthorized", err)
	}
	if _, _, err := auth.ChangePassword(ctx, user.ID, "password123", "short"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("short new password error = %v, want ErrInvalid", err)
	}

	newToken, _, err := auth.ChangePassword(ctx, user.ID, "password123", "newpassword123")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := auth.Authenticate(ctx, oldToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("old token error = %v, want ErrUnauthorized", err)
	}
	if _, err := auth.Authenticate(ctx, newToken); err != nil {
		t.Fatalf("new token: %v", err)
	}
	if len(sessions.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions.sessions))
	}
	if _, _, _, err := auth.Login(ctx, "admin@example.com", "password123"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("old password login error = %v, want ErrUnauthorized", err)
	}
	if _, _, _, err := auth.Login(ctx, "admin@example.com", "newpassword123"); err != nil {
		t.Fatalf("new password login: %v", err)
	}

	if err := auth.DeleteAccount(ctx, user.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := auth.Authenticate(ctx, newToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("deleted account token error = %v, want ErrUnauthorized", err)
	}
	if err := auth.DeleteAccount(ctx, user.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second DeleteAccount error = %v, want ErrNotFound", err)
	}
}
