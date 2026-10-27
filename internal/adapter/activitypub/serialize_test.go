package activitypub

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"biblio-oggetto/internal/domain"
)

func TestMarshalActivityCreate(t *testing.T) {
	published := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	entry := domain.OutboxEntry{
		ID:        "act1",
		Kind:      domain.OutboxCreate,
		Published: published,
		Object: domain.Object{
			ID:          "obj1",
			Name:        "Dune",
			Description: "desert novel",
			ImagePath:   "/static/uploads/dune.png",
			Tags:        []string{"scifi"},
			CreatedAt:   published,
			UpdatedAt:   published,
		},
	}

	body, err := marshalActivity(entry, "https://catalog.example.com/ap/actors/catalog", "https://catalog.example.com")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var activity map[string]any
	if err := json.Unmarshal(body, &activity); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if activity["type"] != "Create" {
		t.Fatalf("activity = %+v", activity)
	}
	if activity["id"] != "https://catalog.example.com/ap/outbox/act1" {
		t.Fatalf("activity id = %v", activity["id"])
	}

	object := activity["object"].(map[string]any)
	if object["id"] != "https://catalog.example.com/ap/objects/obj1" || object["type"] != "Article" {
		t.Fatalf("object = %+v", object)
	}
	if object["image"] != "https://catalog.example.com/static/uploads/dune.png" {
		t.Fatalf("image = %v, want absolutized path", object["image"])
	}
	attachments := object["attachment"].([]any)
	first := attachments[0].(map[string]any)
	if first["content"] != "available" {
		t.Fatalf("availability attachment = %+v", first)
	}
	last := attachments[len(attachments)-1].(map[string]any)
	if last["name"] != "#scifi" {
		t.Fatalf("tag attachment = %+v", last)
	}
	if strings.Contains(string(body), "0001-01-01") {
		t.Fatal("zero timestamps leaked into payload")
	}
}

func TestMarshalActivityDelete(t *testing.T) {
	entry := domain.OutboxEntry{
		ID:        "act2",
		Kind:      domain.OutboxDelete,
		Published: time.Now().UTC(),
		Object:    domain.Object{ID: "obj1", Name: "Dune"},
	}
	body, err := marshalActivity(entry, "https://catalog.example.com/ap/actors/catalog", "https://catalog.example.com")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var activity map[string]any
	if err := json.Unmarshal(body, &activity); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if activity["type"] != "Delete" {
		t.Fatalf("activity = %+v", activity)
	}
	object := activity["object"].(map[string]any)
	if object["type"] != "Tombstone" || object["id"] != "https://catalog.example.com/ap/objects/obj1" {
		t.Fatalf("tombstone = %+v", object)
	}
}

func TestMarshalActivityAccept(t *testing.T) {
	entry := domain.OutboxEntry{
		ID:        "act3",
		Kind:      domain.OutboxAccept,
		Published: time.Now().UTC(),
		Target:    "https://remote.example/alice",
		FollowID:  "https://remote.example/follows/1",
	}
	body, err := marshalActivity(entry, "https://catalog.example.com/ap/actors/catalog", "https://catalog.example.com")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var activity map[string]any
	if err := json.Unmarshal(body, &activity); err != nil {
		t.Fatalf("decode: %v", err)
	}
	object := activity["object"].(map[string]any)
	if object["type"] != "Follow" || object["actor"] != entry.Target || object["id"] != entry.FollowID {
		t.Fatalf("accept object = %+v", object)
	}
}

func TestArticleBusyAvailability(t *testing.T) {
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	article := articleDoc("https://catalog.example.com", domain.Object{
		ID: "obj1", Name: "Dune", Busy: true, BusyUntil: &until,
	})
	if len(article.Attachment) != 1 || article.Attachment[0].Content != "on loan until 2026-11-01T00:00:00Z" {
		t.Fatalf("attachments = %+v", article.Attachment)
	}
}
