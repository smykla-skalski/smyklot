package webhook

import (
	"io"
	"net/http"
)

// receiver answers GitHub. It verifies, screens and claims, and it does all of
// that before any work runs: GitHub gives a delivery ten seconds and does not
// retry one that times out.
type receiver struct {
	pipeline *Pipeline
}

func (rec receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := rec.pipeline
	event := r.Header.Get(EventHeader)

	// GitHub sends a ping when a webhook is first configured and will not send
	// anything else until it is answered.
	if event == EventPing {
		p.opts.received(event, OutcomeIgnored)
		w.WriteHeader(http.StatusOK)

		return
	}

	// An event nobody subscribed to is answered before it costs a parse or a
	// row. Not before the signature check, though - that has already read the
	// body, and answering an unsigned request differently by event would tell
	// whoever sent it which events this deployment listens for.
	if !p.opts.accepts(event) {
		p.opts.received(event, OutcomeIgnored)
		w.WriteHeader(http.StatusNoContent)

		return
	}

	deliveryID := sanitizeDeliveryID(r.Header.Get(DeliveryHeader))
	logger := p.opts.Logger.With(
		"delivery_id", deliveryID,
		"event", eventLabel(event, p.opts.known),
	)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		p.opts.received(event, OutcomeInvalid)
		logger.Error("cannot read body", "error", err)
		http.Error(w, "cannot read body", http.StatusBadRequest)

		return
	}

	source, err := ParseSource(body)
	if err != nil {
		p.opts.received(event, OutcomeInvalid)
		logger.Error("malformed payload", "error", err)
		http.Error(w, "malformed payload", http.StatusBadRequest)

		return
	}

	delivery := Delivery{
		Event:   event,
		ID:      deliveryID,
		Source:  source,
		Payload: body,
		Key:     claimKey(event, deliveryID, body),
		Attempt: 1,
		Logger: logger.With(
			"repo", source.Repository.FullName,
			"action", source.Action,
		),
	}

	if p.opts.Screen != nil {
		wanted, screenErr := p.opts.Screen(delivery)
		if screenErr != nil {
			p.opts.received(event, OutcomeInvalid)
			delivery.Logger.Error("delivery refused by screen", "error", screenErr)
			http.Error(w, "malformed payload", http.StatusBadRequest)

			return
		}
		if !wanted {
			p.opts.received(event, OutcomeIgnored)
			w.WriteHeader(http.StatusNoContent)

			return
		}
	}

	rec.claim(w, r, delivery)
}

// claim takes ownership of a delivery, or explains why it did not.
//
// Claiming before queueing is what makes a redelivery harmless: the second copy
// never reaches a handler.
func (rec receiver) claim(w http.ResponseWriter, r *http.Request, delivery Delivery) {
	p := rec.pipeline

	result, err := p.inbox.Claim(r.Context(), Claim{
		Key:        delivery.Key,
		DeliveryID: delivery.ID,
		Event:      delivery.Event,
		Source:     delivery.Source,
		Payload:    delivery.Payload,
		At:         p.opts.Now(),
	})
	if err != nil {
		p.opts.received(delivery.Event, OutcomeRefused)
		delivery.Logger.Error("delivery claim failed", "error", err)
		http.Error(w, "not accepted", http.StatusServiceUnavailable)

		return
	}

	switch result.Disposition {
	case InProgress:
		p.opts.received(delivery.Event, OutcomeDuplicate)
		delivery.Logger.Info("delivery is still being processed")
		w.WriteHeader(http.StatusAccepted)

	case Retained:
		p.opts.received(delivery.Event, OutcomeDuplicate)
		delivery.Logger.Info("delivery already handled")
		w.WriteHeader(http.StatusOK)

	case Accepted:
		p.wake()
		p.opts.received(delivery.Event, OutcomeAccepted)
		w.WriteHeader(http.StatusAccepted)

	default:
		p.opts.received(delivery.Event, OutcomeRefused)
		delivery.Logger.Error("inbox returned an unknown disposition",
			"disposition", string(result.Disposition))
		http.Error(w, "not accepted", http.StatusServiceUnavailable)
	}
}

// unsigned answers a delivery whose signature did not check out. The handler
// never sees the body, so an unsigned delivery cannot change anything.
func (p *Pipeline) unsigned(w http.ResponseWriter, r *http.Request, err error) {
	p.opts.received(r.Header.Get(EventHeader), OutcomeUnsigned)
	p.opts.Logger.Warn("rejected an unsigned delivery", "error", err)
	http.Error(w, "invalid signature", http.StatusUnauthorized)
}
