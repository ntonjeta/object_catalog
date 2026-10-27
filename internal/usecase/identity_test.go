package usecase

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"biblio-oggetto/internal/domain"
)

func TestActorGeneratesAndPersistsKey(t *testing.T) {
	keys := &fakeInstanceKeys{}
	identity := NewIdentity(keys, "https://catalog.example.com/")

	actor, err := identity.Actor(context.Background())
	if err != nil {
		t.Fatalf("actor: %v", err)
	}
	if actor.ID != "https://catalog.example.com/ap/actors/catalog" {
		t.Fatalf("actor id = %q", actor.ID)
	}
	if actor.Inbox != "https://catalog.example.com/ap/inbox" || actor.Outbox != "https://catalog.example.com/ap/outbox" {
		t.Fatalf("actor endpoints = %q, %q", actor.Inbox, actor.Outbox)
	}
	if actor.PublicKeyID != actor.ID+"#main-key" {
		t.Fatalf("public key id = %q", actor.PublicKeyID)
	}
	if actor.PreferredUsername != domain.InstanceActorHandle {
		t.Fatalf("preferred username = %q", actor.PreferredUsername)
	}
	if !keys.stored || !strings.Contains(keys.key.PrivatePEM, "PRIVATE KEY") {
		t.Fatalf("key not persisted: %+v", keys)
	}
	parsePublicKey(t, actor.PublicKeyPEM)

	second, err := identity.Actor(context.Background())
	if err != nil {
		t.Fatalf("second actor: %v", err)
	}
	if second.PublicKeyPEM != actor.PublicKeyPEM {
		t.Fatal("public key changed between calls, want reuse of stored key")
	}
}

func TestActorUsesRacedKeyAfterConflict(t *testing.T) {
	raced, err := generateInstanceKey()
	if err != nil {
		t.Fatalf("generate raced key: %v", err)
	}
	keys := &fakeInstanceKeys{race: true, raceKey: raced}
	identity := NewIdentity(keys, "https://catalog.example.com")

	actor, err := identity.Actor(context.Background())
	if err != nil {
		t.Fatalf("actor: %v", err)
	}
	privateKey, err := parsePrivatePEM(raced.PrivatePEM)
	if err != nil {
		t.Fatalf("parse raced key: %v", err)
	}
	expected, err := marshalPublicPEM(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal expected public key: %v", err)
	}
	if actor.PublicKeyPEM != expected {
		t.Fatal("actor uses generated key, want the key stored by the racing writer")
	}
}

func TestActorRejectsCorruptStoredKey(t *testing.T) {
	keys := &fakeInstanceKeys{
		stored: true,
		key:    domain.InstanceKey{PrivatePEM: "not a pem", CreatedAt: time.Now()},
	}
	identity := NewIdentity(keys, "https://catalog.example.com")

	if _, err := identity.Actor(context.Background()); err == nil {
		t.Fatal("actor with corrupt stored key succeeded, want error")
	}
}

func parsePublicKey(t *testing.T, publicPEM string) {
	t.Helper()
	block, rest := pem.Decode([]byte(publicPEM))
	if block == nil || len(rest) > 0 || block.Type != "PUBLIC KEY" {
		t.Fatalf("public key is not a single PUBLIC KEY PEM block: %q", publicPEM)
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	if _, ok := parsed.(*rsa.PublicKey); !ok {
		t.Fatalf("public key is %T, want *rsa.PublicKey", parsed)
	}
}
