package e2e

import (
	"encoding/json"
	"net/http"
	"testing"
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
		"/.well-known/webfinger?resource=acct:catalog@"+federationAcctHost, nil)
	assertStatus(t, response, http.StatusOK, data)
	if contentType := response.Header.Get("Content-Type"); contentType != "application/jrd+json" {
		t.Fatalf("content type = %q, want application/jrd+json", contentType)
	}

	var envelope webfingerEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode webfinger: %v; body: %s", err, data)
	}
	actorID := federationBaseURL + "/ap/actors/catalog"
	if envelope.Subject != "acct:catalog@"+federationAcctHost {
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
		"/.well-known/webfinger?resource=ACCT:Catalog@Catalog.Example.COM", nil)
	assertStatus(t, response, http.StatusOK, data)

	cases := []struct {
		name        string
		query       string
		wantStatus  int
		wantErrCode string
	}{
		{name: "wrong host", query: "?resource=acct:catalog@evil.example", wantStatus: http.StatusNotFound, wantErrCode: "not_found"},
		{name: "wrong handle", query: "?resource=acct:admin@catalog.example.com", wantStatus: http.StatusNotFound, wantErrCode: "not_found"},
		{name: "non-acct scheme", query: "?resource=https://catalog.example.com/ap/actors/catalog", wantStatus: http.StatusNotFound, wantErrCode: "not_found"},
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
	actorID := federationBaseURL + "/ap/actors/catalog"

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
	if actor.Inbox != federationBaseURL+"/ap/inbox" || actor.Outbox != federationBaseURL+"/ap/outbox" {
		t.Fatalf("actor endpoints = %q, %q", actor.Inbox, actor.Outbox)
	}
	if actor.PublicKey.ID != actorID+"#main-key" || actor.PublicKey.Owner != actorID {
		t.Fatalf("public key id/owner = %q / %q", actor.PublicKey.ID, actor.PublicKey.Owner)
	}
	if !contains(actor.Context, "https://www.w3.org/ns/activitystreams") {
		t.Fatalf("contexts = %v", actor.Context)
	}
	if actor.PublicKey.PublicKeyPEM == "" {
		t.Fatal("public key pem is empty")
	}

	// Key is stable across requests (persisted, not regenerated).
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

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
