package httpapi

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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, actorID, nil)
	if err != nil {
		return domain.RemoteActor{}, err
	}
	request.Header.Set("Accept", activityJSONContentType)
	response, err := r.client.Do(request)
	if err != nil {
		return domain.RemoteActor{}, fmt.Errorf("fetch actor %q: %w", actorID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return domain.RemoteActor{}, fmt.Errorf("fetch actor %q: status %d", actorID, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
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
