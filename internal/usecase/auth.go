package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"biblio-oggetto/internal/domain"
)

type Auth struct {
	users      UserRepository
	sessions   SessionRepository
	sessionTTL time.Duration
}

func NewAuth(users UserRepository, sessions SessionRepository, sessionTTL time.Duration) *Auth {
	return &Auth{users: users, sessions: sessions, sessionTTL: sessionTTL}
}

// CreateAdmin creates the initial administrator account. It is intended to be
// called from the create-admin command, not from the HTTP API.
func (a *Auth) CreateAdmin(ctx context.Context, email, password string) (domain.User, error) {
	email = domain.NormalizeEmail(email)
	if err := domain.ValidateEmail(email); err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return domain.User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	id, err := domain.NewID()
	if err != nil {
		return domain.User{}, fmt.Errorf("generate user id: %w", err)
	}
	user := domain.User{
		ID:           id,
		Email:        email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}
	if err := a.users.Create(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (a *Auth) Login(ctx context.Context, email, password string) (domain.User, string, time.Time, error) {
	user, err := a.users.ByEmail(ctx, domain.NormalizeEmail(email))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, "", time.Time{}, fmt.Errorf("%w: invalid email or password", domain.ErrUnauthorized)
		}
		return domain.User{}, "", time.Time{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return domain.User{}, "", time.Time{}, fmt.Errorf("%w: invalid email or password", domain.ErrUnauthorized)
	}
	token, hash, err := newSessionToken()
	if err != nil {
		return domain.User{}, "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(a.sessionTTL)
	if err := a.sessions.DeleteExpired(ctx, time.Now().UTC()); err != nil {
		return domain.User{}, "", time.Time{}, err
	}
	if err := a.sessions.Create(ctx, hash, user.ID, expiresAt); err != nil {
		return domain.User{}, "", time.Time{}, err
	}
	return user, token, expiresAt, nil
}

func (a *Auth) Authenticate(ctx context.Context, token string) (domain.User, error) {
	if token == "" {
		return domain.User{}, domain.ErrUnauthorized
	}
	session, err := a.sessions.ByTokenHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, domain.ErrUnauthorized
		}
		return domain.User{}, err
	}
	if !session.ExpiresAt.After(time.Now().UTC()) {
		if err := a.sessions.Delete(ctx, hashToken(token)); err != nil && !errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, err
		}
		return domain.User{}, domain.ErrUnauthorized
	}
	user, err := a.users.ByID(ctx, session.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, domain.ErrUnauthorized
		}
		return domain.User{}, err
	}
	return user, nil
}

func (a *Auth) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := a.sessions.Delete(ctx, hashToken(token)); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}

// TODO check this procedure don't trust the LLM in security as is
func (a *Auth) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) (string, time.Time, error) {
	if err := domain.ValidatePassword(newPassword); err != nil {
		return "", time.Time{}, err
	}
	user, err := a.users.ByID(ctx, userID)
	if err != nil {
		return "", time.Time{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return "", time.Time{}, fmt.Errorf("%w: current password is incorrect", domain.ErrUnauthorized)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("hash password: %w", err)
	}
	if err := a.users.UpdatePassword(ctx, userID, string(hash)); err != nil {
		return "", time.Time{}, err
	}
	if err := a.sessions.DeleteByUser(ctx, userID); err != nil {
		return "", time.Time{}, err
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(a.sessionTTL)
	if err := a.sessions.Create(ctx, tokenHash, userID, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (a *Auth) DeleteAccount(ctx context.Context, userID string) error {
	return a.users.Delete(ctx, userID)
}

func newSessionToken() (token, tokenHash string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b[:])
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
