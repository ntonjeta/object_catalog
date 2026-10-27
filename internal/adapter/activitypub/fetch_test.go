package activitypub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"biblio-oggetto/internal/domain"
)

// writeJSON serves body as JSON with the given content type.
func writeJSON(w http.ResponseWriter, status int, contentType string, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func actorDoc(id string) map[string]any {
	return map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Person",
		"id":       id,
		"inbox":    id + "/inbox",
	}
}

func TestResolveActorURL(t *testing.T) {
	var actorID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ap/actors/catalog" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, activityJSONContentType, actorDoc(actorID))
	}))
	defer ts.Close()

	actorID = ts.URL + "/ap/actors/catalog"
	actor, err := NewRemoteActors().Resolve(context.Background(), actorID)
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", actorID, err)
	}
	if actor.ID != actorID {
		t.Errorf("actor.ID = %q, want %q", actor.ID, actorID)
	}
	if want := actorID + "/inbox"; actor.InboxURL != want {
		t.Errorf("actor.InboxURL = %q, want %q", actor.InboxURL, want)
	}
}

func TestResolveAcctHandleViaWebFinger(t *testing.T) {
	var actorID string
	var gotResource, gotAccept string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/webfinger":
			gotResource = r.URL.Query().Get("resource")
			gotAccept = r.Header.Get("Accept")
			writeJSON(w, http.StatusOK, "application/jrd+json", map[string]any{
				"subject": "acct:catalog@" + r.Host,
				"links": []map[string]string{
					{"rel": "self", "href": actorID},
				},
			})
		case "/ap/actors/catalog":
			writeJSON(w, http.StatusOK, activityJSONContentType, actorDoc(actorID))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://") // 127.0.0.1:port → http scheme rule
	actorID = ts.URL + "/ap/actors/catalog"
	actors := NewRemoteActors()

	// Both "acct:user@host" and the bare "user@host" form must resolve.
	for _, handle := range []string{"acct:catalog@" + host, "catalog@" + host} {
		actor, err := actors.Resolve(context.Background(), "  "+handle+"  ")
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", handle, err)
		}
		if actor.ID != actorID {
			t.Errorf("Resolve(%q): actor.ID = %q, want %q", handle, actor.ID, actorID)
		}
		if want := actorID + "/inbox"; actor.InboxURL != want {
			t.Errorf("Resolve(%q): actor.InboxURL = %q, want %q", handle, actor.InboxURL, want)
		}
	}

	if want := "acct:catalog@" + host; gotResource != want {
		t.Errorf("webfinger resource = %q, want %q", gotResource, want)
	}
	if gotAccept != "application/jrd+json" {
		t.Errorf("webfinger Accept = %q, want application/jrd+json", gotAccept)
	}
}

func TestResolveJunk(t *testing.T) {
	actors := NewRemoteActors()
	for _, input := range []string{"", "not a url", "mailto:catalog@example.com", "catalog", "ftp://example.com/catalog"} {
		_, err := actors.Resolve(context.Background(), input)
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("Resolve(%q) error = %v, want domain.ErrInvalid", input, err)
		}
	}
}

func TestResolveUnknownHandle(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "webfinger 404",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
		},
		{
			name: "empty self href",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusOK, "application/jrd+json", map[string]any{
					"links": []map[string]string{{"rel": "self", "href": ""}},
				})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			defer ts.Close()

			host := strings.TrimPrefix(ts.URL, "http://")
			_, err := NewRemoteActors().Resolve(context.Background(), "acct:catalog@"+host)
			if !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("Resolve error = %v, want domain.ErrNotFound", err)
			}
		})
	}
}
