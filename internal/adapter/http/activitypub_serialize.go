package httpapi

import (
	"encoding/json"
	"strings"
	"time"

	"biblio-oggetto/internal/domain"
)

const (
	asNamespace = "https://www.w3.org/ns/activitystreams"
	asPublic    = "https://www.w3.org/ns/activitystreams#Public"
)

type asActivity struct {
	Context   json.RawMessage `json:"@context"`
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Actor     string          `json:"actor"`
	Published string          `json:"published"`
	To        []string        `json:"to,omitempty"`
	Object    any             `json:"object"`
}

type asCollection struct {
	Context      json.RawMessage   `json:"@context"`
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	TotalItems   int               `json:"totalItems"`
	OrderedItems []json.RawMessage `json:"orderedItems"`
}

type asArticle struct {
	Context    json.RawMessage `json:"@context"`
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	Content    string          `json:"content,omitempty"`
	Image      string          `json:"image,omitempty"`
	Published  string          `json:"published,omitempty"`
	Updated    string          `json:"updated,omitempty"`
	Attachment []asAttachment  `json:"attachment,omitempty"`
}

type asAttachment struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content,omitempty"`
}

type asTombstone struct {
	Context json.RawMessage `json:"@context"`
	ID      string          `json:"id"`
	Type    string          `json:"type"`
}

var asContextJSON = mustContext([]string{asNamespace})

func mustContext(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic("encode activity context: " + err.Error())
	}
	return encoded
}

// objectURL is the canonical ActivityPub id for a catalog object.
func objectURL(baseURL, objectID string) string {
	return baseURL + "/ap/objects/" + objectID
}

func articleDoc(baseURL string, object domain.Object) asArticle {
	article := asArticle{
		Context:   asContextJSON,
		ID:        objectURL(baseURL, object.ID),
		Type:      "Article",
		Name:      object.Name,
		Content:   object.Description,
		Published: object.CreatedAt.Format(time.RFC3339),
		Updated:   object.UpdatedAt.Format(time.RFC3339),
	}
	if strings.HasPrefix(object.ImagePath, "/") {
		article.Image = baseURL + object.ImagePath
	} else {
		article.Image = object.ImagePath
	}
	availability := "available"
	if object.Busy {
		availability = "on loan until " + formatTimeRFC3339(object.BusyUntil)
		if object.BusyUntil == nil {
			availability = "on loan"
		}
	}
	article.Attachment = append(article.Attachment, asAttachment{
		Type: "Note", Name: "Availability", Content: availability,
	})
	if object.Category != nil {
		article.Attachment = append(article.Attachment, asAttachment{
			Type: "Note", Name: "Category", Content: object.Category.Name,
		})
	}
	for _, tag := range object.Tags {
		article.Attachment = append(article.Attachment, asAttachment{
			Type: "Tag", Name: "#" + tag,
		})
	}
	return article
}

func formatTimeRFC3339(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

// marshalActivity renders one outbox entry as an ActivityStreams activity.
func marshalActivity(entry domain.OutboxEntry, actorID, baseURL string) ([]byte, error) {
	activity := asActivity{
		Context:   asContextJSON,
		ID:        baseURL + "/ap/outbox/" + entry.ID,
		Type:      "Announce",
		Actor:     actorID,
		Published: entry.Published.Format(time.RFC3339),
		To:        []string{asPublic},
	}
	switch entry.Kind {
	case domain.OutboxCreate, domain.OutboxUpdate:
		activity.Type = "Create"
		if entry.Kind == domain.OutboxUpdate {
			activity.Type = "Update"
		}
		activity.Object = articleDoc(baseURL, entry.Object)
	case domain.OutboxDelete:
		activity.Type = "Delete"
		activity.Object = asTombstone{
			Context: asContextJSON,
			ID:      objectURL(baseURL, entry.Object.ID),
			Type:    "Tombstone",
		}
	case domain.OutboxAccept:
		activity.Type = "Accept"
		follow := map[string]string{"type": "Follow", "actor": entry.Target, "object": actorID}
		if entry.FollowID != "" {
			follow["id"] = entry.FollowID
		}
		activity.Object = follow
	}
	return json.Marshal(activity)
}
