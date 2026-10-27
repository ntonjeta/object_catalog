package activitypub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"biblio-oggetto/internal/domain"
)

// RemoteActors resolves federated actor documents over plain HTTP.
// Authorized-fetch signatures arrive with inbox verification (PR 3).
type RemoteActors struct {
	client *http.Client
}

func NewRemoteActors() *RemoteActors {
	return &RemoteActors{client: &http.Client{Timeout: 10 * time.Second}}
}

func (r *RemoteActors) FetchActor(ctx context.Context, actorID string) (domain.RemoteActor, error) {
	body, err := r.fetchDocument(ctx, actorID)
	if err != nil {
		return domain.RemoteActor{}, err
	}
	var document struct {
		ID    string `json:"id"`
		Inbox string `json:"inbox"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return domain.RemoteActor{}, fmt.Errorf("decode actor %q: %w", actorID, err)
	}
	if !strings.EqualFold(document.ID, actorID) {
		return domain.RemoteActor{}, fmt.Errorf("actor document id %q does not match requested %q", document.ID, actorID)
	}
	if err := validateAbsoluteHTTPURL(document.Inbox); err != nil {
		return domain.RemoteActor{}, fmt.Errorf("actor %q: %w", actorID, err)
	}
	return domain.RemoteActor{ID: document.ID, InboxURL: document.Inbox}, nil
}

func validateAbsoluteHTTPURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("%w: url is required", domain.ErrInvalid)
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%w: url must be an absolute http(s) URL", domain.ErrInvalid)
	}
	return nil
}

// FetchPublicKey resolves a signature key id to its public key PEM. The key
// id may be an actor id ("https://host/ap/actors/alice"), an actor id with a
// fragment ("...alice#main-key"), or a standalone key document URL.
func (r *RemoteActors) FetchPublicKey(ctx context.Context, keyID string) (string, error) {
	documentURL := keyID
	if i := strings.IndexByte(keyID, '#'); i >= 0 {
		documentURL = keyID[:i]
	}
	body, err := r.fetchDocument(ctx, documentURL)
	if err != nil {
		return "", err
	}
	var document struct {
		PublicKeyPem string `json:"publicKeyPem"`
		Owner        string `json:"owner"`
		PublicKey    struct {
			ID           string `json:"id"`
			PublicKeyPem string `json:"publicKeyPem"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return "", fmt.Errorf("decode key document %q: %w", documentURL, err)
	}
	if document.PublicKey.PublicKeyPem != "" {
		return document.PublicKey.PublicKeyPem, nil
	}
	if document.PublicKeyPem != "" {
		return document.PublicKeyPem, nil
	}
	return "", fmt.Errorf("key document %q carries no publicKeyPem", documentURL)
}

func (r *RemoteActors) fetchDocument(ctx context.Context, documentURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, documentURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", activityJSONContentType)
	response, err := r.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch document %q: %w", documentURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch document %q: status %d", documentURL, response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
}
