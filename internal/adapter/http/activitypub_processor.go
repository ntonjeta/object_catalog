package httpapi

import (
	"context"
	"log"
	"time"

	"biblio-oggetto/internal/usecase"
)

// InboxProcessor drains verified inbox activities. One goroutine, started by
// the composition root alongside the Deliverer.
type InboxProcessor struct {
	federation *usecase.Federation
	poll       time.Duration
}

func NewInboxProcessor(federation *usecase.Federation, poll time.Duration) *InboxProcessor {
	return &InboxProcessor{federation: federation, poll: poll}
}

func (p *InboxProcessor) Run(ctx context.Context) {
	ticker := time.NewTicker(p.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.federation.ProcessPendingInbox(ctx, time.Now().UTC()); err != nil {
				log.Printf("federation: process inbox: %v", err)
			}
		}
	}
}
