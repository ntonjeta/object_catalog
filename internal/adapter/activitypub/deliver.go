package activitypub

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-fed/httpsig"

	"biblio-oggetto/internal/domain"
	"biblio-oggetto/internal/usecase"
)

const (
	deliveryTimeout = 10 * time.Second
)

// Deliverer polls the federation outbox and POSTS pending activities with
// HTTP signatures: broadcasts to follower inboxes, follow and undo entries
// to their target peer's inbox. One goroutine, started by the composition
// root.
type Deliverer struct {
	federation *usecase.Federation
	identity   *usecase.Identity
	client     *http.Client
	poll       time.Duration
}

func NewDeliverer(federation *usecase.Federation, identity *usecase.Identity, poll time.Duration) *Deliverer {
	return &Deliverer{
		federation: federation,
		identity:   identity,
		client:     &http.Client{Timeout: deliveryTimeout},
		poll:       poll,
	}
}

func (d *Deliverer) Run(ctx context.Context) {
	ticker := time.NewTicker(d.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.process(ctx)
		}
	}
}

func (d *Deliverer) process(ctx context.Context) {
	now := time.Now().UTC()
	entries, err := d.federation.PendingDeliveries(ctx, now)
	if err != nil {
		log.Printf("federation: pending outbox: %v", err)
		return
	}
	if len(entries) == 0 {
		return
	}
	followers, err := d.federation.Followers(ctx)
	if err != nil {
		log.Printf("federation: list followers: %v", err)
		return
	}
	actor, err := d.identity.Actor(ctx)
	if err != nil {
		log.Printf("federation: load instance actor: %v", err)
		return
	}
	privateKey, keyID, err := d.identity.SigningKey(ctx)
	if err != nil {
		log.Printf("federation: load signing key: %v", err)
		return
	}
	signer, _, err := httpsig.NewSigner(
		[]httpsig.Algorithm{httpsig.RSA_SHA256},
		httpsig.DigestSha256,
		[]string{httpsig.RequestTarget, "host", "date", "digest"},
		httpsig.Signature,
		0,
	)
	if err != nil {
		log.Printf("federation: create signer: %v", err)
		return
	}

	for _, entry := range entries {
		body, err := marshalActivity(entry, actor.ID, d.identity.BaseURL())
		if err != nil {
			log.Printf("federation: encode entry %s: %v", entry.ID, err)
			d.fail(ctx, entry, now)
			continue
		}
		// A follow or undo addresses exactly one instance: its target peer.
		// Never let the zero-follower fast path consume these entries — an
		// instance with no followers must still be able to join peers.
		if entry.Kind == domain.OutboxFollow || entry.Kind == domain.OutboxUndo {
			target, err := d.federation.FetchActor(ctx, entry.Target)
			if err != nil {
				log.Printf("federation: resolve target %s for entry %s: %v", entry.Target, entry.ID, err)
				d.fail(ctx, entry, now)
				continue
			}
			follower := domain.Follower{ActorID: target.ID, InboxURL: target.InboxURL}
			if err := d.post(ctx, signer, privateKey, keyID, follower, body); err != nil {
				log.Printf("federation: deliver %s to %s: %v", entry.ID, follower.ActorID, err)
				d.fail(ctx, entry, now)
				continue
			}
			if err := d.federation.MarkDelivered(ctx, entry.ID, now); err != nil {
				log.Printf("federation: mark delivered %s: %v", entry.ID, err)
			}
			continue
		}
		if len(followers) == 0 {
			if err := d.federation.MarkDelivered(ctx, entry.ID, now); err != nil {
				log.Printf("federation: mark delivered %s: %v", entry.ID, err)
			}
			continue
		}
		delivered := true
		for _, follower := range followers {
			if err := d.post(ctx, signer, privateKey, keyID, follower, body); err != nil {
				delivered = false
				log.Printf("federation: deliver %s to %s: %v", entry.ID, follower.ActorID, err)
			}
		}
		if delivered {
			if err := d.federation.MarkDelivered(ctx, entry.ID, now); err != nil {
				log.Printf("federation: mark delivered %s: %v", entry.ID, err)
			}
			continue
		}
		d.fail(ctx, entry, now)
	}
}

func (d *Deliverer) fail(ctx context.Context, entry domain.OutboxEntry, now time.Time) {
	attempts := entry.Attempts + 1
	if err := d.federation.MarkAttempt(ctx, entry.ID, attempts, now.Add(usecase.RetryDelay(attempts))); err != nil {
		log.Printf("federation: record attempt %s: %v", entry.ID, err)
	}
}

func (d *Deliverer) post(ctx context.Context, signer httpsig.Signer, privateKey any, keyID string, follower domain.Follower, body []byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, follower.InboxURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", activityJSONContentType)
	request.Header.Set("Host", request.URL.Host)
	request.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	if err := signer.SignRequest(privateKey, keyID, request, body); err != nil {
		return fmt.Errorf("sign request: %w", err)
	}
	response, err := d.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("inbox %s answered %s", follower.InboxURL, response.Status)
	}
	return nil
}
