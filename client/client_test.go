// This file is part of REANA.
// Copyright (C) 2026 CERN.

package client

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"reanahub/reana-client-go/client/operations"

	"github.com/spf13/viper"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return function(request)
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (body *trackedBody) Close() error {
	body.closed = true
	return nil
}

func TestBoundedResponseBodyRejectsBytesPastLimit(t *testing.T) {
	body := &boundedResponseBody{
		body:      io.NopCloser(strings.NewReader("four")),
		remaining: 3,
	}
	contents, err := io.ReadAll(body)
	if err == nil ||
		!strings.Contains(err.Error(), ErrResponseTooLarge.Error()) {
		t.Fatalf("expected response limit error, got %q and %v", contents, err)
	}
	if !bytes.Equal(contents, []byte("fou")) {
		t.Fatalf("unexpected bounded contents %q", contents)
	}
}

func TestBoundedResponseBodyAcceptsExactLimit(t *testing.T) {
	body := &boundedResponseBody{
		body:      io.NopCloser(strings.NewReader("three")),
		remaining: 5,
	}
	contents, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, []byte("three")) {
		t.Fatalf("unexpected contents %q", contents)
	}
}

func TestBoundedResponseBodyClose(t *testing.T) {
	underlying := &trackedBody{Reader: strings.NewReader("")}
	body := &boundedResponseBody{body: underlying, remaining: 1}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if !underlying.closed {
		t.Error("expected Close to be forwarded")
	}
}

func TestBoundedResponseTransport(t *testing.T) {
	t.Run("rejects declared oversized response", func(t *testing.T) {
		body := &trackedBody{Reader: strings.NewReader("response")}
		transport := &boundedResponseTransport{
			transport: roundTripFunc(func(
				*http.Request,
			) (*http.Response, error) {
				return &http.Response{
					Body:          body,
					ContentLength: maxAPIResponseBytes + 1,
				}, nil
			}),
		}
		_, err := transport.RoundTrip(
			httptest.NewRequest(http.MethodGet, "https://reana.test", nil),
		)
		if err == nil ||
			!strings.Contains(err.Error(), ErrResponseTooLarge.Error()) {
			t.Fatalf("expected response limit error, got %v", err)
		}
		if !body.closed {
			t.Error("expected rejected response body to be closed")
		}
	})

	t.Run("bounds response with unknown length", func(t *testing.T) {
		transport := &boundedResponseTransport{
			transport: roundTripFunc(func(
				*http.Request,
			) (*http.Response, error) {
				return &http.Response{
					Body:          io.NopCloser(strings.NewReader("response")),
					ContentLength: -1,
				}, nil
			}),
		}
		response, err := transport.RoundTrip(
			httptest.NewRequest(http.MethodGet, "https://reana.test", nil),
		)
		if err != nil {
			t.Fatal(err)
		}
		bounded, ok := response.Body.(*boundedResponseBody)
		if !ok || bounded.remaining != maxAPIResponseBytes {
			t.Fatalf("response body was not bounded: %#v", response.Body)
		}
	})

	t.Run("propagates transport error", func(t *testing.T) {
		want := errors.New("connection failed")
		transport := &boundedResponseTransport{
			transport: roundTripFunc(func(
				*http.Request,
			) (*http.Response, error) {
				return nil, want
			}),
		}
		_, err := transport.RoundTrip(
			httptest.NewRequest(http.MethodGet, "https://reana.test", nil),
		)
		if !errors.Is(err, want) {
			t.Fatalf("expected %v, got %v", want, err)
		}
	})
}

func TestAPIClientRejectsInvalidServerURL(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		serverURL string
		errorText string
	}{
		{"missing", "", "REANA_SERVER_URL is not set"},
		{"relative", "/api", "REANA_SERVER_URL is not set"},
		{"unsupported scheme", "ftp://reana.test", "unsupported"},
		{"malformed", "https://[::1", "missing ']'"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			viper.Set("server-url", testCase.serverURL)
			t.Cleanup(viper.Reset)
			_, err := ApiClient()
			if err == nil ||
				!strings.Contains(err.Error(), testCase.errorText) {
				t.Fatalf("expected %q error, got %v", testCase.errorText, err)
			}
		})
	}
}

func TestAPIClientBoundsControlResponses(t *testing.T) {
	payload := `{"status":"` +
		strings.Repeat("x", maxAPIResponseBytes) +
		`"}`
	for _, testCase := range []struct {
		name          string
		contentLength bool
	}{
		{"known length", true},
		{"chunked", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewTLSServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if testCase.contentLength {
						w.Header().Set(
							"Content-Length",
							strconv.Itoa(len(payload)),
						)
					} else {
						w.WriteHeader(http.StatusOK)
						w.(http.Flusher).Flush()
					}
					_, _ = w.Write([]byte(payload))
				}),
			)
			defer server.Close()
			viper.Set("server-url", server.URL)
			defer viper.Reset()

			api, err := ControlAPIClient()
			if err != nil {
				t.Fatal(err)
			}
			token := "token"
			params := operations.NewGetWorkflowStatusParams()
			params.SetAccessToken(&token)
			params.SetWorkflowIDOrName("analysis")
			_, err = api.Operations.GetWorkflowStatus(params)
			if err == nil ||
				!strings.Contains(err.Error(), ErrResponseTooLarge.Error()) {
				t.Fatalf("expected bounded validation response, got %v", err)
			}
		})
	}
}

func TestAPIClientConnectsToSelfSignedTLSServer(t *testing.T) {
	server := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"running"}`))
		}),
	)
	defer server.Close()
	viper.Set("server-url", server.URL)
	t.Cleanup(viper.Reset)

	defaultTransport := http.DefaultTransport.(*http.Transport)
	defaultInsecureSkipVerify := false
	if defaultTransport.TLSClientConfig != nil {
		defaultInsecureSkipVerify =
			defaultTransport.TLSClientConfig.InsecureSkipVerify
	}
	api, err := ApiClient()
	if err != nil {
		t.Fatal(err)
	}
	token := "token"
	params := operations.NewGetWorkflowStatusParams()
	params.SetAccessToken(&token)
	params.SetWorkflowIDOrName("analysis")
	response, err := api.Operations.GetWorkflowStatus(params)
	if err != nil {
		t.Fatal(err)
	}
	if response.Payload.Status != "running" {
		t.Errorf("unexpected status %q", response.Payload.Status)
	}
	if defaultTransport.TLSClientConfig != nil &&
		defaultTransport.TLSClientConfig.InsecureSkipVerify !=
			defaultInsecureSkipVerify {
		t.Error("ApiClient disabled TLS verification on the default transport")
	}
}
