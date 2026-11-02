package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"biblio-oggetto/internal/domain"
)

// Ingest caps clamp peer announcements to the domain limits; keep them in
// sync with internal/domain (ValidateObjectName, ValidateObjectDescription,
// ValidateCategoryName, NormalizeTags). Mirrored content is truncated rather
// than rejected: a legitimate peer's oversized field must not wedge the
// inbox queue.
const (
	maxRemoteNameLength         = 200  // domain.ValidateObjectName cap
	maxRemoteDescriptionLength  = 5000 // domain.ValidateObjectDescription cap
	maxRemoteCategoryNameLength = 100  // domain.ValidateCategoryName cap
	maxRemoteTags               = 20   // domain.NormalizeTags cap
	maxRemoteTagLength          = 50   // domain.NormalizeTags per-tag cap
)

const (
	availabilityOnLoan       = "on loan"
	availabilityOnLoanPrefix = "on loan until "
)

// processAccept marks the peer whose Follow activity this Accept answers.
// Accepts we cannot match — an unknown follow activity, or one sent by an
// actor other than the peer that owns the follow — are dropped without error.
func (f *Federation) processAccept(ctx context.Context, event domain.InboxEvent) error {
	peer, err := f.peers.ByFollowActivityID(ctx, event.ObjectID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find peer for follow activity %q: %w", event.ObjectID, err)
	}
	if peer.ActorID != event.Actor {
		return nil
	}
	return f.peers.MarkAccepted(ctx, peer.ActorID)
}

// ingestArticle mirrors the object a Create/Update from an accepted peer
// announces. An article that does not parse is dropped and logged: known
// content failures must not retry forever, only repository failures do.
func (f *Federation) ingestArticle(ctx context.Context, event domain.InboxEvent) error {
	object, ok := decodePeerArticle([]byte(event.Payload), event.Actor, event.ReceivedAt)
	if !ok {
		log.Printf("federation: dropping unparseable article %q from peer %q", event.ObjectID, event.Actor)
		return nil
	}
	return f.remoteObjects.PutFromAcceptedPeer(ctx, object)
}

// peerActivityEnvelope is the part of an activity JSON document ingest needs
// before touching the announced object. The article shape mirrors the
// serializer's output (adapter/activitypub serialize.go): the full Article
// is embedded in "object"; the url field is accepted and ignored.
type peerActivityEnvelope struct {
	Actor     string          `json:"actor"`
	Published string          `json:"published"`
	Object    json.RawMessage `json:"object"`
}

type peerArticle struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Content    string            `json:"content"`
	Image      string            `json:"image"`
	Published  string            `json:"published"`
	Updated    string            `json:"updated"`
	Attachment []peerArticlePart `json:"attachment"`
}

type peerArticlePart struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

// decodePeerArticle parses a biblio-oggetto Create/Update activity from an
// accepted peer into a RemoteObject. The second result reports whether the
// article may be ingested; it is false when the payload is unparseable, the
// "object" carries no article (e.g. a bare id string), the activity actor
// does not match peerActorID, the article id is missing or not an absolute
// http(s) URL, the article id fails the origin rule (its scheme+host must
// equal the peer actor's, case-insensitively), or the name is empty after
// truncation. Everything else is clamped or cleared, never rejected. Missing
// article timestamps fall back to the activity's published time, then receipt.
func decodePeerArticle(payload []byte, peerActorID string, receivedAt time.Time) (domain.RemoteObject, bool) {
	var envelope peerActivityEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return domain.RemoteObject{}, false
	}
	if envelope.Actor != peerActorID || len(envelope.Object) == 0 {
		return domain.RemoteObject{}, false
	}
	var article peerArticle
	if err := json.Unmarshal(envelope.Object, &article); err != nil {
		return domain.RemoteObject{}, false
	}
	articleID := strings.TrimSpace(article.ID)
	if err := validateRemoteURL("article id", articleID); err != nil {
		return domain.RemoteObject{}, false
	}
	if !sameOrigin(articleID, peerActorID) {
		return domain.RemoteObject{}, false
	}
	name := truncateRunes(strings.TrimSpace(article.Name), maxRemoteNameLength)
	if name == "" {
		return domain.RemoteObject{}, false
	}

	object := domain.RemoteObject{
		ID:          articleID,
		PeerActorID: peerActorID,
		Name:        name,
		Description: truncateRunes(article.Content, maxRemoteDescriptionLength),
		CreatedAt:   parsePeerTime(article.Published),
		UpdatedAt:   parsePeerTime(article.Updated),
	}
	if image := strings.TrimSpace(article.Image); validateRemoteURL("image url", image) == nil {
		object.ImageURL = image
	}
	if object.CreatedAt.IsZero() {
		object.CreatedAt = object.UpdatedAt
	}
	if object.UpdatedAt.IsZero() {
		object.UpdatedAt = object.CreatedAt
	}
	fallback := parsePeerTime(envelope.Published)
	if fallback.IsZero() {
		fallback = receivedAt.UTC()
	}
	if fallback.IsZero() {
		fallback = time.Now().UTC()
	}
	if object.CreatedAt.IsZero() {
		object.CreatedAt = fallback
	}
	if object.UpdatedAt.IsZero() {
		object.UpdatedAt = fallback
	}

	var tags []string
	for _, part := range article.Attachment {
		switch part.Type {
		case "Note":
			switch part.Name {
			case "Availability":
				object.Busy, object.BusyUntil = parseAvailability(part.Content)
			case "Category":
				object.CategoryName = truncateRunes(strings.TrimSpace(part.Content), maxRemoteCategoryNameLength)
			}
		case "Tag":
			tags = append(tags, strings.TrimPrefix(part.Name, "#"))
		}
	}
	object.Tags = normalizeIngestTags(tags)
	return object, true
}

// sameOrigin reports whether two URLs share scheme and host, compared
// case-insensitively. This is the federation origin rule: a peer may only
// announce objects whose id lives on the peer's own origin.
func sameOrigin(a, b string) bool {
	originA, okA := urlOrigin(a)
	originB, okB := urlOrigin(b)
	return okA && okB && strings.EqualFold(originA, originB)
}

func urlOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	return parsed.Scheme + "://" + parsed.Host, true
}

// parseAvailability maps the Availability attachment onto the mirrored busy
// state. Any trimmed "on loan..." wording fails closed unless it has a valid
// "on loan until <RFC3339>" expiry: malformed or missing timestamps report
// Busy with a nil expiry, because under-reporting a loan would show a lent-out
// item as free. Anything without the loan wording reads as available.
func parseAvailability(content string) (bool, *time.Time) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, availabilityOnLoan) {
		return false, nil
	}
	expiry := strings.TrimPrefix(content, availabilityOnLoanPrefix)
	if expiry == content {
		return true, nil
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(expiry))
	if err != nil {
		return true, nil
	}
	utc := until.UTC()
	return true, &utc
}

// normalizeIngestTags clamps a peer's tag list to the domain caps before
// domain.NormalizeTags runs. Deduplication happens first so the 20-tag cap
// counts unique tags — duplicates early in the list must not crowd out new
// tags past position 20. Per-tag length truncation keeps NormalizeTags from
// ever erroring; if it somehow does, the tags are dropped entirely rather
// than failing the ingest.
func normalizeIngestTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	unique := make([]string, 0, len(tags))
	for _, tag := range tags {
		key := strings.ToLower(strings.TrimSpace(tag))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, tag)
	}
	if len(unique) > maxRemoteTags {
		unique = unique[:maxRemoteTags]
	}
	for i, tag := range unique {
		unique[i] = truncateRunes(tag, maxRemoteTagLength)
	}
	normalized, err := domain.NormalizeTags(unique)
	if err != nil || len(normalized) == 0 {
		return nil
	}
	return normalized
}

func parsePeerTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
