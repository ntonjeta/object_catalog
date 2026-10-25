package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"biblio-oggetto/internal/domain"
)

type fakeUsers struct {
	users   map[string]domain.User
	byEmail map[string]string
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{
		users:   make(map[string]domain.User),
		byEmail: make(map[string]string),
	}
}

func (f *fakeUsers) Create(_ context.Context, user domain.User) error {
	if _, ok := f.byEmail[user.Email]; ok {
		return domain.ErrConflict
	}
	f.users[user.ID] = user
	f.byEmail[user.Email] = user.ID
	return nil
}

func (f *fakeUsers) ByEmail(_ context.Context, email string) (domain.User, error) {
	id, ok := f.byEmail[email]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return f.users[id], nil
}

func (f *fakeUsers) ByID(_ context.Context, id string) (domain.User, error) {
	user, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}

func (f *fakeUsers) UpdatePassword(_ context.Context, id, passwordHash string) error {
	user, ok := f.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	user.PasswordHash = passwordHash
	f.users[id] = user
	return nil
}

func (f *fakeUsers) Delete(_ context.Context, id string) error {
	user, ok := f.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	delete(f.users, id)
	delete(f.byEmail, user.Email)
	return nil
}

type fakeSession struct {
	userID    string
	expiresAt time.Time
}

type fakeSessions struct {
	sessions map[string]fakeSession
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{sessions: make(map[string]fakeSession)}
}

func (f *fakeSessions) Create(_ context.Context, tokenHash, userID string, expiresAt time.Time) error {
	f.sessions[tokenHash] = fakeSession{userID: userID, expiresAt: expiresAt}
	return nil
}

func (f *fakeSessions) ByTokenHash(_ context.Context, tokenHash string) (domain.Session, error) {
	session, ok := f.sessions[tokenHash]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}
	return domain.Session{UserID: session.userID, ExpiresAt: session.expiresAt}, nil
}

func (f *fakeSessions) Delete(_ context.Context, tokenHash string) error {
	if _, ok := f.sessions[tokenHash]; !ok {
		return domain.ErrNotFound
	}
	delete(f.sessions, tokenHash)
	return nil
}

func (f *fakeSessions) DeleteByUser(_ context.Context, userID string) error {
	for hash, session := range f.sessions {
		if session.userID == userID {
			delete(f.sessions, hash)
		}
	}
	return nil
}

func (f *fakeSessions) DeleteExpired(_ context.Context, now time.Time) error {
	for hash, session := range f.sessions {
		if !session.expiresAt.After(now) {
			delete(f.sessions, hash)
		}
	}
	return nil
}

type fakeCategories struct {
	categories map[string]domain.Category
	lastID     string
}

func newFakeCategories() *fakeCategories {
	return &fakeCategories{categories: make(map[string]domain.Category)}
}

func (f *fakeCategories) Create(_ context.Context, category domain.Category) error {
	for _, existing := range f.categories {
		if strings.EqualFold(existing.Name, category.Name) {
			return domain.ErrConflict
		}
	}
	f.categories[category.ID] = category
	return nil
}

func (f *fakeCategories) List(_ context.Context) ([]domain.Category, error) {
	categories := make([]domain.Category, 0, len(f.categories))
	for _, category := range f.categories {
		categories = append(categories, category)
	}
	return categories, nil
}

func (f *fakeCategories) ByID(_ context.Context, id string) (domain.Category, error) {
	category, ok := f.categories[id]
	if !ok {
		return domain.Category{}, domain.ErrNotFound
	}
	return category, nil
}

func (f *fakeCategories) Update(_ context.Context, category domain.Category) error {
	if _, ok := f.categories[category.ID]; !ok {
		return domain.ErrNotFound
	}
	f.categories[category.ID] = category
	return nil
}

func (f *fakeCategories) Delete(_ context.Context, id string) error {
	if _, ok := f.categories[id]; !ok {
		return domain.ErrNotFound
	}
	delete(f.categories, id)
	return nil
}

type fakeObjects struct {
	objects    map[string]domain.Object
	lastFilter ObjectFilter
	createErr  error
	updateErr  error
}

func newFakeObjects() *fakeObjects {
	return &fakeObjects{objects: make(map[string]domain.Object)}
}

func (f *fakeObjects) Create(_ context.Context, object domain.Object) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.objects[object.ID] = object
	return nil
}

func (f *fakeObjects) Update(_ context.Context, object domain.Object) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if _, ok := f.objects[object.ID]; !ok {
		return domain.ErrNotFound
	}
	f.objects[object.ID] = object
	return nil
}

func (f *fakeObjects) Delete(_ context.Context, id string) error {
	if _, ok := f.objects[id]; !ok {
		return domain.ErrNotFound
	}
	delete(f.objects, id)
	return nil
}

func (f *fakeObjects) ByID(_ context.Context, id string) (domain.Object, error) {
	object, ok := f.objects[id]
	if !ok {
		return domain.Object{}, domain.ErrNotFound
	}
	return object, nil
}

func (f *fakeObjects) List(_ context.Context, filter ObjectFilter) ([]domain.Object, int, error) {
	f.lastFilter = filter
	objects := make([]domain.Object, 0, len(f.objects))
	for _, object := range f.objects {
		objects = append(objects, object)
	}
	return objects, len(objects), nil
}

type fakePhotos struct {
	next    int
	saved   []string
	deleted []string
}

func newFakePhotos() *fakePhotos {
	return &fakePhotos{}
}

func (f *fakePhotos) Save(_ context.Context, _ PhotoUpload) (string, error) {
	f.next++
	path := fmt.Sprintf("photo-%d.png", f.next)
	f.saved = append(f.saved, path)
	return path, nil
}

func (f *fakePhotos) Delete(_ context.Context, path string) error {
	f.deleted = append(f.deleted, path)
	return nil
}
