package webhook

import (
	"io"
	"net/http"
)

type receiver struct {
	pipeline *Pipeline
}

func (rec receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := rec.pipeline
	event := r.Header.Get(EventHeader)

	if event == EventPing {
		p.opts.received(event, OutcomeIgnored)
		w.WriteHeader(http.StatusOK)

		return
	}

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
			p.opts.decorate(delivery).Logger.Error(
				"delivery refused by screen", "error", screenErr,
			)
			http.Error(w, "malformed payload", http.StatusBadRequest)

			return
		}
		if !wanted {
			p.opts.received(event, OutcomeIgnored)
			w.WriteHeader(http.StatusNoContent)

			return
		}
	}

	rec.claim(w, r, p.opts.decorate(delivery))
}

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

func (p *Pipeline) unsigned(w http.ResponseWriter, r *http.Request, err error) {
	event := r.Header.Get(EventHeader)

	p.opts.received(event, OutcomeUnsigned)
	p.opts.Logger.Warn("rejected an unsigned delivery",
		"delivery_id", sanitizeDeliveryID(r.Header.Get(DeliveryHeader)),
		"event", eventLabel(event, p.opts.known),
		"error", err)
	http.Error(w, "invalid signature", http.StatusUnauthorized)
}
