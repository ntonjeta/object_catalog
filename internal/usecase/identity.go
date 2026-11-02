package usecase

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"biblio-oggetto/internal/domain"
)

const (
	instanceActorName  = "Object Catalog"
	instanceKeyBits    = 2048
	instanceActorRoute = "/ap/actors/"
)

// InstanceActor is the federation identity of this instance, with the RSA
// public key derived from the stored private key.
type InstanceActor struct {
	ID                string
	Name              string
	PreferredUsername string
	Inbox             string
	Outbox            string
	PublicKeyID       string
	PublicKeyPEM      string
}

type Identity struct {
	keys    InstanceKeyRepository
	baseURL string
}

func NewIdentity(keys InstanceKeyRepository, baseURL string) *Identity {
	return &Identity{keys: keys, baseURL: strings.TrimRight(baseURL, "/")}
}

// BaseURL is the validated canonical origin the actor identifiers derive from.
func (i *Identity) BaseURL() string { return i.baseURL }

// SigningKey loads (or creates) the instance key and returns the private key
// with its public-key identifier.
func (i *Identity) SigningKey(ctx context.Context) (*rsa.PrivateKey, string, error) {
	key, err := i.loadOrCreateKey(ctx)
	if err != nil {
		return nil, "", err
	}
	privateKey, err := parsePrivatePEM(key.PrivatePEM)
	if err != nil {
		return nil, "", err
	}
	keyID := i.baseURL + instanceActorRoute + domain.InstanceActorHandle + "#main-key"
	return privateKey, keyID, nil
}

// Actor loads or creates the instance signing key and returns the actor
// document data.
func (i *Identity) Actor(ctx context.Context) (InstanceActor, error) {
	key, err := i.loadOrCreateKey(ctx)
	if err != nil {
		return InstanceActor{}, err
	}
	privateKey, err := parsePrivatePEM(key.PrivatePEM)
	if err != nil {
		return InstanceActor{}, err
	}
	publicPEM, err := marshalPublicPEM(&privateKey.PublicKey)
	if err != nil {
		return InstanceActor{}, err
	}

	id := i.baseURL + instanceActorRoute + domain.InstanceActorHandle
	return InstanceActor{
		ID:                id,
		Name:              instanceActorName,
		PreferredUsername: domain.InstanceActorHandle,
		Inbox:             i.baseURL + "/ap/inbox",
		Outbox:            i.baseURL + "/ap/outbox",
		PublicKeyID:       id + "#main-key",
		PublicKeyPEM:      publicPEM,
	}, nil
}

func (i *Identity) loadOrCreateKey(ctx context.Context) (domain.InstanceKey, error) {
	key, err := i.keys.Get(ctx)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.InstanceKey{}, err
	}
	key, err = generateInstanceKey()
	if err != nil {
		return domain.InstanceKey{}, err
	}
	if err := i.keys.Create(ctx, key); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return i.keys.Get(ctx)
		}
		return domain.InstanceKey{}, err
	}
	return key, nil
}

func generateInstanceKey() (domain.InstanceKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, instanceKeyBits)
	if err != nil {
		return domain.InstanceKey{}, fmt.Errorf("generate instance key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return domain.InstanceKey{}, fmt.Errorf("encode instance key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return domain.InstanceKey{PrivatePEM: string(pemBytes), CreatedAt: time.Now().UTC()}, nil
}

func parsePrivatePEM(value string) (*rsa.PrivateKey, error) {
	block, rest := pem.Decode([]byte(value))
	if block == nil || len(rest) > 0 || block.Type != "PRIVATE KEY" {
		return nil, errors.New("instance key is not a PEM PRIVATE KEY block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse instance key: %w", err)
	}
	privateKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("instance key is not an RSA key")
	}
	return privateKey, nil
}

func marshalPublicPEM(publicKey *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("encode public key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}
