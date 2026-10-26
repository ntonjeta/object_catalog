package e2e

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestObjectCRUDAndSearch(t *testing.T) {
	environment := newEnv(t)
	environment.login()
	category := environment.createCategory("Books")

	dune := environment.createObject(map[string]any{
		"name":        "  Dune  ",
		"description": "A desert planet",
		"category_id": category.ID,
		"tags":        []string{"SciFi", "classic", "scifi", "  "},
	})
	if dune.Name != "Dune" {
		t.Fatalf("name = %q, want %q", dune.Name, "Dune")
	}
	if dune.Category == nil || dune.Category.ID != category.ID {
		t.Fatalf("category = %+v, want %s", dune.Category, category.ID)
	}
	if !equalStrings(dune.Tags, []string{"scifi", "classic"}) {
		t.Fatalf("tags = %v, want [scifi classic]", dune.Tags)
	}

	foundation := environment.createObject(map[string]any{
		"name":        "Foundation",
		"description": "A galactic empire falls",
		"tags":        []string{"classic"},
	})
	sequel := environment.createObject(map[string]any{
		"name":        "Dune Messiah",
		"description": "The aftermath",
		"tags":        []string{"sequel"},
	})

	response, data := environment.do(environment.anonymous, http.MethodGet, "/api/objects/"+dune.ID, nil)
	assertStatus(t, response, http.StatusOK, data)
	if object := decode[objectEnvelope](t, data).Object; object.Name != "Dune" {
		t.Fatalf("object name = %q, want Dune", object.Name)
	}

	tests := []struct {
		name    string
		query   url.Values
		wantIDs []string
	}{
		{name: "all", query: nil, wantIDs: []string{sequel.ID, foundation.ID, dune.ID}},
		{name: "name", query: url.Values{"q": {"dune"}}, wantIDs: []string{sequel.ID, dune.ID}},
		{name: "description", query: url.Values{"q": {"desert"}}, wantIDs: []string{dune.ID}},
		{name: "category", query: url.Values{"category": {"books"}}, wantIDs: []string{dune.ID}},
		{name: "category via q", query: url.Values{"q": {"books"}}, wantIDs: []string{dune.ID}},
		{name: "tag", query: url.Values{"tag": {"CLASSIC"}}, wantIDs: []string{foundation.ID, dune.ID}},
		{name: "tag via q", query: url.Values{"q": {"scifi"}}, wantIDs: []string{dune.ID}},
		{name: "tag and query", query: url.Values{"q": {"dune"}, "tag": {"sequel"}}, wantIDs: []string{sequel.ID}},
		{name: "no match", query: url.Values{"q": {"nothing"}}, wantIDs: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := "/api/objects"
			if len(test.query) > 0 {
				path += "?" + test.query.Encode()
			}
			response, data := environment.do(environment.anonymous, http.MethodGet, path, nil)
			assertStatus(t, response, http.StatusOK, data)
			envelope := decode[objectsEnvelope](t, data)
			got := make([]string, 0, len(envelope.Objects))
			for _, object := range envelope.Objects {
				got = append(got, object.ID)
			}
			if strings.Join(got, ",") != strings.Join(test.wantIDs, ",") {
				t.Fatalf("ids = %v, want %v", got, test.wantIDs)
			}
			if envelope.Total != len(test.wantIDs) {
				t.Fatalf("total = %d, want %d", envelope.Total, len(test.wantIDs))
			}
		})
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects?limit=1&offset=1", nil)
	assertStatus(t, response, http.StatusOK, data)
	page := decode[objectsEnvelope](t, data)
	if len(page.Objects) != 1 || page.Total != 3 || page.Limit != 1 || page.Offset != 1 {
		t.Fatalf("page = %+v, want one object, total 3, limit 1, offset 1", page)
	}

	for _, query := range []string{"limit=-1", "limit=101", "offset=-1", "limit=nope"} {
		response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects?"+query, nil)
		assertStatus(t, response, http.StatusBadRequest, data)
		assertErrorCode(t, data, "invalid_input")
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects/missing", nil)
	assertStatus(t, response, http.StatusNotFound, data)
}

func TestObjectUpdateDeleteAndCategoryDetach(t *testing.T) {
	environment := newEnv(t)
	environment.login()
	category := environment.createCategory("Books")
	object := environment.createObject(map[string]any{
		"name":        "Dune",
		"description": "desert",
		"category_id": category.ID,
		"tags":        []string{"scifi"},
	})

	response, data := environment.do(environment.admin, http.MethodPatch, "/api/objects/"+object.ID, map[string]any{
		"name":        "Dune Messiah",
		"description": "aftermath",
		"tags":        []string{"Sequel", "classic", "sequel"},
	})
	assertStatus(t, response, http.StatusOK, data)
	updated := decode[objectEnvelope](t, data).Object
	if updated.Name != "Dune Messiah" || updated.Description != "aftermath" {
		t.Fatalf("updated object = %+v", updated)
	}
	if !equalStrings(updated.Tags, []string{"sequel", "classic"}) {
		t.Fatalf("tags = %v, want [sequel classic]", updated.Tags)
	}
	if updated.Category == nil || updated.Category.ID != category.ID {
		t.Fatalf("category = %+v, want %s", updated.Category, category.ID)
	}

	response, data = environment.do(environment.admin, http.MethodPatch, "/api/objects/"+object.ID, map[string]any{
		"category_id": "",
	})
	assertStatus(t, response, http.StatusOK, data)
	if cleared := decode[objectEnvelope](t, data).Object; cleared.Category != nil {
		t.Fatalf("category = %+v, want nil", cleared.Category)
	}

	response, data = environment.do(environment.admin, http.MethodPatch, "/api/objects/missing", map[string]string{
		"name": "Missing",
	})
	assertStatus(t, response, http.StatusNotFound, data)

	response, data = environment.do(environment.admin, http.MethodDelete, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusNoContent, data)

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusNotFound, data)

	response, data = environment.do(environment.admin, http.MethodDelete, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusNotFound, data)

	remaining := environment.createObject(map[string]any{
		"name":        "Hyperion",
		"category_id": category.ID,
	})
	response, data = environment.do(environment.admin, http.MethodDelete, "/api/categories/"+category.ID, nil)
	assertStatus(t, response, http.StatusNoContent, data)
	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects/"+remaining.ID, nil)
	assertStatus(t, response, http.StatusOK, data)
	if detached := decode[objectEnvelope](t, data).Object; detached.Category != nil {
		t.Fatalf("category = %+v, want nil after category deletion", detached.Category)
	}
}

func TestObjectBusyState(t *testing.T) {
	environment := newEnv(t)
	environment.login()
	object := environment.createObject(map[string]any{"name": "Dune"})
	path := "/api/objects/" + object.ID + "/busy"
	until := time.Now().UTC().Add(time.Hour).Truncate(time.Second)

	response, data := environment.do(environment.anonymous, http.MethodPatch, path, map[string]any{
		"busy":       true,
		"busy_until": until,
	})
	assertStatus(t, response, http.StatusUnauthorized, data)

	response, data = environment.do(environment.admin, http.MethodPatch, path, map[string]any{
		"busy":       true,
		"busy_until": until,
	})
	assertStatus(t, response, http.StatusOK, data)
	busy := decode[objectEnvelope](t, data).Object
	if !busy.Busy || busy.BusyUntil == nil || !busy.BusyUntil.Equal(until) {
		t.Fatalf("busy object = %+v, want busy until %v", busy, until)
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusOK, data)
	public := decode[objectEnvelope](t, data).Object
	if !public.Busy || public.BusyUntil == nil {
		t.Fatalf("public object = %+v, want busy", public)
	}

	for _, body := range []map[string]any{
		{"busy": true},
		{"busy": true, "busy_until": time.Now().UTC().Add(-time.Hour)},
	} {
		response, data = environment.do(environment.admin, http.MethodPatch, path, body)
		assertStatus(t, response, http.StatusBadRequest, data)
		assertErrorCode(t, data, "invalid_input")
	}

	response, data = environment.do(environment.admin, http.MethodPatch, "/api/objects/missing/busy", map[string]any{
		"busy": false,
	})
	assertStatus(t, response, http.StatusNotFound, data)

	response, data = environment.do(environment.admin, http.MethodPatch, path, map[string]any{
		"busy": false,
	})
	assertStatus(t, response, http.StatusOK, data)
	released := decode[objectEnvelope](t, data).Object
	if released.Busy || released.BusyUntil != nil {
		t.Fatalf("released object = %+v, want available", released)
	}

	past := time.Now().UTC().Add(-time.Hour)
	if _, err := environment.app.DB.Exec(
		`UPDATE objects SET busy = 1, busy_until = ? WHERE id = ?`,
		past.Format(time.RFC3339Nano), object.ID,
	); err != nil {
		t.Fatalf("set expired busy: %v", err)
	}
	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusOK, data)
	expired := decode[objectEnvelope](t, data).Object
	if expired.Busy || expired.BusyUntil != nil {
		t.Fatalf("expired object = %+v, want available", expired)
	}
}

func TestObjectReservation(t *testing.T) {
	environment := newEnv(t)
	environment.login()
	object := environment.createObject(map[string]any{"name": "Dune"})
	path := "/api/objects/" + object.ID + "/reservation"
	now := time.Now().UTC()
	from := now.Add(24 * time.Hour).Truncate(time.Second)
	until := now.Add(48 * time.Hour).Truncate(time.Second)

	response, data := environment.do(environment.anonymous, http.MethodPatch, path, map[string]any{
		"from":  from,
		"until": until,
	})
	assertStatus(t, response, http.StatusUnauthorized, data)

	response, data = environment.do(environment.admin, http.MethodPatch, path, map[string]any{
		"from":  from,
		"until": until,
	})
	assertStatus(t, response, http.StatusOK, data)
	reserved := decode[objectEnvelope](t, data).Object
	if reserved.ReservedFrom == nil || !reserved.ReservedFrom.Equal(from) || reserved.Busy {
		t.Fatalf("reserved object = %+v, want reserved from %v and not busy", reserved, from)
	}

	response, data = environment.do(environment.anonymous, http.MethodGet, "/api/objects/"+object.ID, nil)
	assertStatus(t, response, http.StatusOK, data)
	public := decode[objectEnvelope](t, data).Object
	if public.Busy || public.ReservedFrom == nil {
		t.Fatalf("public object = %+v, want reserved and available", public)
	}

	for _, body := range []map[string]any{
		{"from": now.Add(-time.Hour)},
		{"from": from, "until": now.Add(-time.Hour)},
		{"from": from, "until": from},
	} {
		response, data = environment.do(environment.admin, http.MethodPatch, path, body)
		assertStatus(t, response, http.StatusBadRequest, data)
		assertErrorCode(t, data, "invalid_input")
	}

	response, data = environment.do(environment.admin, http.MethodPatch, "/api/objects/missing/reservation", map[string]any{
		"from": from,
	})
	assertStatus(t, response, http.StatusNotFound, data)

	busyObject := environment.createObject(map[string]any{"name": "Crimeq"})
	response, data = environment.do(environment.admin, http.MethodPatch, "/api/objects/"+busyObject.ID+"/busy", map[string]any{
		"busy":       true,
		"busy_until": until,
	})
	assertStatus(t, response, http.StatusOK, data)
	response, data = environment.do(environment.admin, http.MethodPatch, "/api/objects/"+busyObject.ID+"/reservation", map[string]any{
		"from": from,
	})
	assertStatus(t, response, http.StatusConflict, data)
	assertErrorCode(t, data, "conflict")

	response, data = environment.do(environment.admin, http.MethodPatch, "/api/objects/"+object.ID+"/busy", map[string]any{
		"busy": false,
	})
	assertStatus(t, response, http.StatusOK, data)
	released := decode[objectEnvelope](t, data).Object
	if released.Busy || released.ReservedFrom != nil {
		t.Fatalf("released object = %+v, want available", released)
	}
}

func TestObjectValidation(t *testing.T) {
	environment := newEnv(t)
	environment.login()

	tests := []struct {
		name   string
		body   any
		status int
		code   string
	}{
		{name: "missing name", body: map[string]any{"description": "x"}, status: http.StatusBadRequest, code: "invalid_input"},
		{name: "long description", body: map[string]any{"name": "x", "description": strings.Repeat("a", 5001)}, status: http.StatusBadRequest, code: "invalid_input"},
		{name: "long tag", body: map[string]any{"name": "x", "tags": []string{strings.Repeat("a", 51)}}, status: http.StatusBadRequest, code: "invalid_input"},
		{name: "unknown category", body: map[string]any{"name": "x", "category_id": "missing"}, status: http.StatusNotFound, code: "not_found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, data := environment.do(environment.admin, http.MethodPost, "/api/objects", test.body)
			assertStatus(t, response, test.status, data)
			assertErrorCode(t, data, test.code)
		})
	}

	tooManyTags := make([]string, 0, 21)
	for i := 0; i < 21; i++ {
		tooManyTags = append(tooManyTags, fmt.Sprintf("tag-%d", i))
	}
	response, data := environment.do(environment.admin, http.MethodPost, "/api/objects", map[string]any{
		"name": "x",
		"tags": tooManyTags,
	})
	assertStatus(t, response, http.StatusBadRequest, data)

	response, data = environment.doRaw(environment.admin, http.MethodPost, "/api/objects", []byte("{"))
	assertStatus(t, response, http.StatusBadRequest, data)

	response, data = environment.doRaw(environment.admin, http.MethodPost, "/api/objects", []byte(`{"name":"x"} {"name":"y"}`))
	assertStatus(t, response, http.StatusBadRequest, data)
}
