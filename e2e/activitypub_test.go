package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"biblio-oggetto/internal/domain"
)

type webfingerEnvelope struct {
	Subject string `json:"subject"`
	Aliases []string
	Links   []struct {
		Rel  string `json:"rel"`
		Type string `json:"type"`
		Href string `json:"href"`
	}
}

type actorEnvelope struct {
	Context           []string `json:"@context"`
	ID                string   `json:"id"`
	Type              string   `json:"type"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferredUsername"`
	Inbox             string   `json:"inbox"`
	Outbox            string   `json:"outbox"`
	PublicKey         struct {
		ID           string `json:"id"`
		Owner        string `json:"owner"`
		PublicKeyPEM string `json:"publicKeyPem"`
	} `json:"publicKey"`
}

func TestWebFingerResolvesInstanceActor(t *testing.T) {
	e := newEnv(t)

	response, data := e.do(e.anonymous, http.MethodGet,
		"/.well-known/webfinger?resource=acct:catalog@"+e.acctHost, nil)
	assertStatus(t, response, http.StatusOK, data)
	if contentType := response.Header.Get("Content-Type"); contentType != "application/jrd+json" {
		t.Fatalf("content type = %q, want application/jrd+json", contentType)
	}

	var envelope webfingerEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode webfinger: %v; body: %s", err, data)
	}
	actorID := e.baseURL + "/ap/actors/catalog"
	if envelope.Subject != "acct:catalog@"+e.acctHost {
		t.Fatalf("subject = %q", envelope.Subject)
	}
	if len(envelope.Aliases) != 1 || envelope.Aliases[0] != actorID {
		t.Fatalf("aliases = %v, want [%s]", envelope.Aliases, actorID)
	}
	if len(envelope.Links) != 1 {
		t.Fatalf("links = %v", envelope.Links)
	}
	link := envelope.Links[0]
	if link.Rel != "self" || link.Type != "application/activity+json" || link.Href != actorID {
		t.Fatalf("link = %+v", link)
	}
}

func TestWebFingerMatchingRules(t *testing.T) {
	e := newEnv(t)

	response, data := e.do(e.anonymous, http.MethodGet,
		"/.well-known/webfinger?resource=ACCT:Catalog@"+e.acctHost, nil)
	assertStatus(t, response, http.StatusOK, data)

	cases := []struct {
		name        string
		query       string
		wantStatus  int
		wantErrCode string
	}{
		{name: "wrong host", query: "?resource=acct:catalog@evil.example", wantStatus: http.StatusNotFound, wantErrCode: "not_found"},
		{name: "wrong handle", query: "?resource=acct:admin@" + e.acctHost, wantStatus: http.StatusNotFound, wantErrCode: "not_found"},
		{name: "non-acct scheme", query: "?resource=" + e.baseURL + "/ap/actors/catalog", wantStatus: http.StatusNotFound, wantErrCode: "not_found"},
		{name: "missing resource", query: "", wantStatus: http.StatusBadRequest, wantErrCode: "invalid_input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, data := e.do(e.anonymous, http.MethodGet, "/.well-known/webfinger"+tc.query, nil)
			assertStatus(t, response, tc.wantStatus, data)
			assertErrorCode(t, data, tc.wantErrCode)
		})
	}
}

func TestInstanceActorDocument(t *testing.T) {
	e := newEnv(t)
	actorID := e.baseURL + "/ap/actors/catalog"

	response, data := e.do(e.anonymous, http.MethodGet, "/ap/actors/catalog", nil)
	assertStatus(t, response, http.StatusOK, data)
	if contentType := response.Header.Get("Content-Type"); contentType != "application/activity+json" {
		t.Fatalf("content type = %q, want application/activity+json", contentType)
	}

	var actor actorEnvelope
	if err := json.Unmarshal(data, &actor); err != nil {
		t.Fatalf("decode actor: %v; body: %s", err, data)
	}
	if actor.ID != actorID || actor.Type != "Service" || actor.PreferredUsername != "catalog" {
		t.Fatalf("actor = %+v", actor)
	}
	if actor.Inbox != e.baseURL+"/ap/inbox" || actor.Outbox != e.baseURL+"/ap/outbox" {
		t.Fatalf("actor endpoints = %q, %q", actor.Inbox, actor.Outbox)
	}
	if actor.PublicKey.ID != actorID+"#main-key" || actor.PublicKey.Owner != actorID {
		t.Fatalf("public key id/owner = %q / %q", actor.PublicKey.ID, actor.PublicKey.Owner)
	}
	if !containsStringLocal(actor.Context, "https://www.w3.org/ns/activitystreams") {
		t.Fatalf("contexts = %v", actor.Context)
	}
	if actor.PublicKey.PublicKeyPEM == "" {
		t.Fatal("public key pem is empty")
	}

	_, second := e.do(e.anonymous, http.MethodGet, "/ap/actors/catalog", nil)
	var secondActor actorEnvelope
	if err := json.Unmarshal(second, &secondActor); err != nil {
		t.Fatalf("decode second actor: %v", err)
	}
	if secondActor.PublicKey.PublicKeyPEM != actor.PublicKey.PublicKeyPEM {
		t.Fatal("public key changed between requests")
	}
}

func TestInstanceActorUnknownHandle(t *testing.T) {
	e := newEnv(t)

	response, data := e.do(e.anonymous, http.MethodGet, "/ap/actors/nobody", nil)
	assertStatus(t, response, http.StatusNotFound, data)
	assertErrorCode(t, data, "not_found")
}

func TestOutboxAnnouncesObjectChanges(t *testing.T) {
	e := newEnv(t)
	e.login()

	created := e.createObject(map[string]any{"name": "Dune", "description": "desert novel"})

	response, data := e.do(e.anonymous, http.MethodGet, "/ap/outbox", nil)
	assertStatus(t, response, http.StatusOK, data)
	var collection struct {
		Type         string `json:"type"`
		TotalItems   int    `json:"totalItems"`
		OrderedItems []struct {
			Type   string `json:"type"`
			Actor  string `json:"actor"`
			Object struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"object"`
		} `json:"orderedItems"`
	}
	if err := json.Unmarshal(data, &collection); err != nil {
		t.Fatalf("decode outbox: %v; body: %s", err, data)
	}
	if collection.Type != "OrderedCollection" || collection.TotalItems != 1 || len(collection.OrderedItems) != 1 {
		t.Fatalf("outbox = %+v; body: %s", collection, data)
	}
	item := collection.OrderedItems[0]
	if item.Type != "Create" || item.Object.Name != "Dune" {
		t.Fatalf("item = %+v", item)
	}
	if item.Actor != e.baseURL+"/ap/actors/catalog" {
		t.Fatalf("actor = %q", item.Actor)
	}

	response, data = e.do(e.anonymous, http.MethodGet, "/ap/objects/"+created.ID, nil)
	assertStatus(t, response, http.StatusOK, data)
	var article struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &article); err != nil {
		t.Fatalf("decode article: %v", err)
	}
	if article.ID != e.baseURL+"/ap/objects/"+created.ID || article.Type != "Article" || article.Name != "Dune" {
		t.Fatalf("article = %+v", article)
	}

	response, data = e.do(e.anonymous, http.MethodGet, "/ap/objects/missing", nil)
	assertStatus(t, response, http.StatusNotFound, data)
}

func TestInboxFollowAndOutboxDelivery(t *testing.T) {
	target := newEnv(t)
	follower := newEnv(t)
	ctx := context.Background()

	targetActor := target.baseURL + "/ap/actors/catalog"
	followerActor := follower.baseURL + "/ap/actors/catalog"

	// Follow the target instance actor, signed with the follower key.
	response, data := follower.signedInboxPost(target, map[string]any{
		"type":   "Follow",
		"id":     follower.baseURL + "/ap/outbox/follow-1",
		"actor":  followerActor,
		"object": targetActor,
	})
	assertStatus(t, response, http.StatusAccepted, data)

	followers := []domain.Follower(nil)
	follower.eventually("target records the follower", func() bool {
		var err error
		followers, err = target.app.Federation.Followers(ctx)
		return err == nil && len(followers) == 1
	})
	if followers[0].ActorID != followerActor || followers[0].InboxURL != follower.baseURL+"/ap/inbox" {
		t.Fatalf("follower = %+v", followers[0])
	}

	// Object creation enqueues a Create activity; the worker delivers it to
	// the follower inbox together with the Accept.
	target.login()
	created := target.createObject(map[string]any{"name": "Dune"})

	var createID string
	follower.eventually("create entry queued", func() bool {
		entries, _, err := target.app.Federation.Outbox(ctx, 0, 0)
		if err != nil {
			return false
		}
		if len(entries) != 2 {
			return false
		}
		for _, entry := range entries {
			if entry.Kind == domain.OutboxCreate && entry.Object.ID == created.ID {
				createID = entry.ID
			}
		}
		return createID != ""
	})

	deadline := time.Now().Add(20 * time.Second)
	for {
		entry, err := target.app.Federation.OutboxEntry(ctx, createID)
		if err != nil {
			t.Fatalf("outbox entry: %v", err)
		}
		if entry.DeliveredAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("create activity was not delivered within 20s")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Undo removes the follower.
	response, data = follower.signedInboxPost(target, map[string]any{
		"type":   "Undo",
		"id":     follower.baseURL + "/ap/outbox/undo-1",
		"actor":  followerActor,
		"object": targetActor,
	})
	assertStatus(t, response, http.StatusAccepted, data)
	target.eventually("follower removed", func() bool {
		stored, err := target.app.Federation.Followers(ctx)
		return err == nil && len(stored) == 0
	})
}

func TestInboxRejectsUnsigned(t *testing.T) {
	target := newEnv(t)
	follower := newEnv(t)

	response, data := target.do(follower.anonymous, http.MethodPost, "/ap/inbox", map[string]any{
		"type":   "Follow",
		"id":     follower.baseURL + "/ap/outbox/follow-1",
		"actor":  follower.baseURL + "/ap/actors/catalog",
		"object": target.baseURL + "/ap/actors/catalog",
	})
	assertStatus(t, response, http.StatusUnauthorized, data)
	assertErrorCode(t, data, "unauthorized")

	followers, err := target.app.Federation.Followers(context.Background())
	if err != nil || len(followers) != 0 {
		t.Fatalf("followers = (%+v, %v), want none", followers, err)
	}
}

func TestInboxRejectsTamperedBody(t *testing.T) {
	target := newEnv(t)
	follower := newEnv(t)

	body, err := json.Marshal(map[string]any{
		"type":   "Follow",
		"id":     follower.baseURL + "/ap/outbox/follow-1",
		"actor":  follower.baseURL + "/ap/actors/catalog",
		"object": target.baseURL + "/ap/actors/catalog",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	response, data := follower.signedInboxPostRaw(target, body, true)
	assertStatus(t, response, http.StatusUnauthorized, data)
	assertErrorCode(t, data, "unauthorized")
}

func TestInboxRejectsSignerActorMismatch(t *testing.T) {
	target := newEnv(t)
	follower := newEnv(t)

	response, data := follower.signedInboxPost(target, map[string]any{
		"type":   "Follow",
		"id":     follower.baseURL + "/ap/outbox/follow-1",
		"actor":  "https://victim.example/actors/someone",
		"object": target.baseURL + "/ap/actors/catalog",
	})
	assertStatus(t, response, http.StatusUnauthorized, data)
	assertErrorCode(t, data, "unauthorized")
}

func TestInboxDeduplicatesActivityIds(t *testing.T) {
	target := newEnv(t)
	follower := newEnv(t)
	ctx := context.Background()

	activity := map[string]any{
		"type":   "Follow",
		"id":     follower.baseURL + "/ap/outbox/follow-1",
		"actor":  follower.baseURL + "/ap/actors/catalog",
		"object": target.baseURL + "/ap/actors/catalog",
	}
	response, data := follower.signedInboxPost(target, activity)
	assertStatus(t, response, http.StatusAccepted, data)
	response, data = follower.signedInboxPost(target, activity)
	assertStatus(t, response, http.StatusAccepted, data)

	total := -1
	follower.eventually("exactly one accept queued", func() bool {
		_, count, err := target.app.Federation.Outbox(ctx, 0, 0)
		if err != nil {
			return false
		}
		total = count
		return count == 1
	})
	if total != 1 {
		t.Fatalf("outbox total = %d, want 1", total)
	}
}

func TestInboxRejectsSignedButUnparseableBody(t *testing.T) {
	target := newEnv(t)
	follower := newEnv(t)

	response, data := follower.signedInboxPostRaw(target, []byte(`{"type":`), false)
	assertStatus(t, response, http.StatusBadRequest, data)
	assertErrorCode(t, data, "invalid_input")
}

func TestInboxIgnoresForeignTarget(t *testing.T) {
	e := newEnv(t)
	follower := newEnv(t)

	response, data := follower.signedInboxPost(e, map[string]any{
		"type":   "Follow",
		"id":     "https://remote.example/follows/1",
		"actor":  follower.baseURL + "/ap/actors/catalog",
		"object": "https://other.example/actors/someone",
	})
	assertStatus(t, response, http.StatusAccepted, data)

	followers, err := e.app.Federation.Followers(context.Background())
	if err != nil || len(followers) != 0 {
		t.Fatalf("followers = (%+v, %v), want none", followers, err)
	}
}

func containsStringLocal(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
