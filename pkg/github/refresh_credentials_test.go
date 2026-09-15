package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRefreshingClientUsesCurrentCredentialForEachRequest(t *testing.T) {
	credential := "first"
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"id":1,"login":"user"}`))
	}))
	defer server.Close()
	client, err := NewRefreshingClient(func() (string, error) { return credential, nil }, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetUser(t.Context(), "user"); err != nil {
		t.Fatal(err)
	}
	credential = "renewed"
	if _, err = client.GetUser(t.Context(), "user"); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "token first" || seen[1] != "token renewed" {
		t.Fatalf("headers=%v", seen)
	}
}

func TestRetryRefreshesCredentialsAfterWaiting(t *testing.T) {
	credential := "first"
	var seen []string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seen = append(seen, req.Header.Get("Authorization"))
		status := http.StatusOK
		if len(seen) == 1 {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
	})
	transport := retryTransport{base: authTransport{base: base, refresh: func() (string, error) { return credential, nil }}, sleep: func(*http.Request, time.Duration) error { credential = "renewed"; return nil }}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if len(seen) != 2 || seen[0] != "token first" || seen[1] != "token renewed" {
		t.Fatalf("headers=%v", seen)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("request headers mutated")
	}
}

func TestRefreshFailureNeverSendsStaleCredentials(t *testing.T) {
	for _, test := range []struct {
		name, token string
		err         error
	}{
		{"empty", "", nil}, {"mint failed", "", errors.New("mint failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := authTransport{token: "stale", refresh: func() (string, error) { return test.token, test.err }, base: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("request sent with failed refresh")
				return nil, nil
			})}
			body := &refreshTestBody{Reader: strings.NewReader("payload")}
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://api.github.com/user", body)
			if _, err := transport.RoundTrip(req); err == nil {
				t.Fatal("refresh error lost")
			}
			if !body.closed {
				t.Fatal("request body leaked on refresh failure")
			}
		})
	}
}

func TestCancelledRequestDoesNotMintCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	transport := authTransport{refresh: func() (string, error) { t.Error("minted after cancellation"); return "", nil }}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if _, err := transport.RoundTrip(req); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

type refreshTestBody struct {
	io.Reader
	closed bool
}

func (b *refreshTestBody) Close() error { b.closed = true; return nil }
