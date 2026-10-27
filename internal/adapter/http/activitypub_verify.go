package httpapi

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-fed/httpsig"

	"biblio-oggetto/internal/domain"
)

const maxSignatureSkew = 12 * time.Hour

// RemoteKeyStore caches remote public keys by their signature key id.
// Implemented by the SQLite adapter; declared here because the verifier is
// the only consumer.
type RemoteKeyStore interface {
	Get(ctx context.Context, keyID string) (string, error)
	Put(ctx context.Context, keyID, publicPEM string, at time.Time) error
}

// SignatureVerifier validates draft-cavage HTTP signatures on incoming
// inbox requests (rsa-sha256 over (request-target) host date digest).
type SignatureVerifier struct {
	store  RemoteKeyStore
	actors *RemoteActors
}

func NewSignatureVerifier(store RemoteKeyStore, actors *RemoteActors) *SignatureVerifier {
	return &SignatureVerifier{store: store, actors: actors}
}

// Verify returns the actor id the signature key belongs to, or an error if
// the request is not authentically signed for its body.
func (v *SignatureVerifier) Verify(r *http.Request, body []byte) (string, error) {
	verifier, err := httpsig.NewVerifier(r)
	if err != nil {
		return "", fmt.Errorf("malformed signature: %w", err)
	}
	if !containsToken(signatureCoveredHeaders(r.Header.Get("Signature")), "digest") {
		return "", errors.New("signature must cover the Digest header")
	}
	if err := checkDateSkew(r.Header.Get("Date"), time.Now()); err != nil {
		return "", err
	}
	if !digestMatches(r.Header.Get("Digest"), body) {
		return "", errors.New("digest does not match body")
	}

	keyID := verifier.KeyId()
	cached := true
	publicPEM, err := v.store.Get(r.Context(), keyID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return "", err
		}
		cached = false
		if publicPEM, err = v.actors.FetchPublicKey(r.Context(), keyID); err != nil {
			return "", err
		}
		v.cacheKey(r.Context(), keyID, publicPEM)
	}

	publicKey, err := parsePublicPEM(publicPEM)
	if err != nil {
		return "", err
	}
	if err := verifier.Verify(publicKey, httpsig.RSA_SHA256); err != nil {
		if !cached {
			return "", fmt.Errorf("signature verification failed: %w", err)
		}
		// Key rotation: refetch once and retry with fresh material.
		fresh, fetchErr := v.actors.FetchPublicKey(r.Context(), keyID)
		if fetchErr != nil {
			return "", fmt.Errorf("signature verification failed: %w", err)
		}
		v.cacheKey(r.Context(), keyID, fresh)
		rotateKey, parseErr := parsePublicPEM(fresh)
		if parseErr != nil {
			return "", parseErr
		}
		if err := verifier.Verify(rotateKey, httpsig.RSA_SHA256); err != nil {
			return "", fmt.Errorf("signature verification failed: %w", err)
		}
	}

	if i := strings.IndexByte(keyID, '#'); i >= 0 {
		return keyID[:i], nil
	}
	return keyID, nil
}

func (v *SignatureVerifier) cacheKey(ctx context.Context, keyID, publicPEM string) {
	if err := v.store.Put(ctx, keyID, publicPEM, time.Now().UTC()); err != nil {
		// Cache write failure is not fatal for this request.
		_ = err
	}
}

func parsePublicPEM(publicPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicPEM))
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("remote key is not a PEM PUBLIC KEY block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse remote public key: %w", err)
	}
	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("remote key is not an RSA key")
	}
	return publicKey, nil
}

func checkDateSkew(date string, now time.Time) error {
	if date == "" {
		return errors.New("missing Date header")
	}
	sent, err := http.ParseTime(date)
	if err != nil {
		return fmt.Errorf("invalid Date header: %w", err)
	}
	skew := now.Sub(sent)
	if skew < 0 {
		skew = -skew
	}
	if skew > maxSignatureSkew {
		return fmt.Errorf("Date header skewed by %v", skew.Round(time.Second))
	}
	return nil
}

func digestMatches(header string, body []byte) bool {
	algo, value, ok := strings.Cut(header, "=")
	if !ok || !strings.EqualFold(algo, "sha-256") {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(body)
	return subtle.ConstantTimeCompare(want, sum[:]) == 1
}

// signatureCoveredHeaders extracts the headers list from a Signature header.
func signatureCoveredHeaders(raw string) []string {
	i := strings.Index(raw, `headers="`)
	if i < 0 {
		return nil
	}
	rest := raw[i+len(`headers="`):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return nil
	}
	return strings.Fields(rest[:j])
}

func containsToken(tokens []string, want string) bool {
	for _, token := range tokens {
		if strings.EqualFold(token, want) {
			return true
		}
	}
	return false
}
