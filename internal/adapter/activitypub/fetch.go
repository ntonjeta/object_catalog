package activitypub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
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

const jrdJSONContentType = "application/jrd+json"

// handlePattern matches "user@host" or "acct:user@host". Colons and slashes
// are excluded from the user so scheme-ful inputs (mailto:, https://) never
// match; the host may contain a port ("127.0.0.1:8080").
var handlePattern = regexp.MustCompile(`^(?:acct:)?([^:@/\s]+)@([^@\s/]+)$`)

// Resolve turns admin join input — "handle@host", "acct:handle@host" or a
// direct actor URL — into the remote actor document. Handles resolve through
// WebFinger to the rel=self actor id first.
func (r *RemoteActors) Resolve(ctx context.Context, input string) (domain.RemoteActor, error) {
	target := strings.TrimSpace(input)
	if match := handlePattern.FindStringSubmatch(target); match != nil {
		actorID, err := r.webfingerSelfURL(ctx, match[1]+"@"+match[2])
		if err != nil {
			return domain.RemoteActor{}, err
		}
		return r.FetchActor(ctx, actorID)
	}
	if parsed, err := url.Parse(target); err == nil && parsed.Host != "" &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") {
		if parsed.User != nil || parsed.Fragment != "" {
			return domain.RemoteActor{}, fmt.Errorf("%w: target must be an actor URL or handle@host", domain.ErrInvalid)
		}
		return r.FetchActor(ctx, target)
	}
	return domain.RemoteActor{}, fmt.Errorf("%w: target must be an actor URL or handle@host", domain.ErrInvalid)
}

// webfingerScheme returns the scheme used to probe host's WebFinger endpoint.
// Plain http is allowed only for the exact host "localhost" or a loopback IP
// (127.0.0.0/8 or ::1), port-insensitive, so local test instances join
// without TLS. Registerable look-alikes (localhost.evil.com,
// 127.0.0.1.evil.com, localhost1) are not loopback and require https. Edge
// cases, documented for the callers: a bracketed IPv6 literal must carry a
// port ("[::1]:8080" is http, bare "[::1]" falls back to https); a bare
// unbracketed "::1" is treated as http but fails later URL parsing when the
// request is built, which maps to ErrInvalid.
func webfingerScheme(host string) string {
	if bare, _, err := net.SplitHostPort(host); err == nil {
		host = bare
	}
	if host == "localhost" {
		return "http"
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return "http"
	}
	return "https"
}

// webfingerSelfURL performs RFC 7033 WebFinger for "user@host" and returns
// the href of the first rel=self link.
func (r *RemoteActors) webfingerSelfURL(ctx context.Context, userAndHost string) (string, error) {
	host := userAndHost[strings.IndexByte(userAndHost, '@')+1:]
	endpoint := fmt.Sprintf("%s://%s/.well-known/webfinger?resource=%s",
		webfingerScheme(host), host, url.QueryEscape("acct:"+userAndHost))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("%w: webfinger target %q: %v", domain.ErrInvalid, userAndHost, err)
	}
	request.Header.Set("Accept", jrdJSONContentType)
	response, err := r.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("webfinger for %q: %w", userAndHost, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("webfinger for %q: status %d: %w", userAndHost, response.StatusCode, domain.ErrNotFound)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("webfinger for %q: %w", userAndHost, err)
	}
	var jrd struct {
		Links []struct {
			Rel  string `json:"rel"`
			Href string `json:"href"`
		} `json:"links"`
	}
	if err := json.Unmarshal(body, &jrd); err != nil {
		return "", fmt.Errorf("decode webfinger for %q: %w", userAndHost, err)
	}
	for _, link := range jrd.Links {
		if link.Rel == "self" && link.Href != "" {
			return link.Href, nil
		}
	}
	return "", fmt.Errorf("webfinger for %q: no self link: %w", userAndHost, domain.ErrNotFound)
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
