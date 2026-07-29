// Package client provides the automatically generated API client, provided by the swagger tool.
package client

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	httptransport "github.com/go-openapi/runtime/client"
)

const (
	apiRequestTimeout   = 5 * time.Minute
	maxAPIResponseBytes = 16 * 1024 * 1024
)

// ErrResponseTooLarge identifies a bounded control-plane response overflow.
var ErrResponseTooLarge = errors.New(
	"REANA server response exceeds 16 MiB limit",
)

type boundedResponseBody struct {
	body      io.ReadCloser
	remaining int64
}

func (body *boundedResponseBody) Read(buffer []byte) (int, error) {
	if body.remaining == 0 {
		var probe [1]byte
		read, err := body.body.Read(probe[:])
		if read > 0 {
			return 0, ErrResponseTooLarge
		}
		return 0, err
	}
	if int64(len(buffer)) > body.remaining {
		buffer = buffer[:body.remaining]
	}
	read, err := body.body.Read(buffer)
	body.remaining -= int64(read)
	return read, err
}

func (body *boundedResponseBody) Close() error {
	return body.body.Close()
}

type boundedResponseTransport struct {
	transport http.RoundTripper
}

// knownLengthMultipartTransport gives generated multipart requests an exact
// Content-Length. go-openapi streams multipart bodies through an io.Pipe, which
// otherwise makes Go send them chunked; deployed uWSGI post-buffering does not
// deliver such request bodies to the application.
type knownLengthMultipartTransport struct {
	transport http.RoundTripper
}

func (transport *knownLengthMultipartTransport) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	mediaType, _, _ := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if request.Body == nil || request.Body == http.NoBody ||
		request.ContentLength > 0 || mediaType != "multipart/form-data" {
		return transport.transport.RoundTrip(request)
	}

	body, err := os.CreateTemp("", "reana-multipart-*")
	if err != nil {
		return nil, fmt.Errorf("could not spool multipart request: %w", err)
	}
	path := body.Name()
	defer func() {
		_ = body.Close()
		_ = os.Remove(path)
	}()

	length, copyErr := io.Copy(body, request.Body)
	closeErr := request.Body.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("could not spool multipart request: %w", copyErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf(
			"could not close multipart request: %w",
			closeErr,
		)
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("could not rewind multipart request: %w", err)
	}

	request.Body = body
	request.ContentLength = length
	request.TransferEncoding = nil
	request.Header.Del("Transfer-Encoding")
	if length == 0 {
		request.Body = http.NoBody
	}
	return transport.transport.RoundTrip(request)
}

func (transport *boundedResponseTransport) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	response, err := transport.transport.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.ContentLength > maxAPIResponseBytes {
		_ = response.Body.Close()
		return nil, ErrResponseTooLarge
	}
	response.Body = &boundedResponseBody{
		body:      response.Body,
		remaining: maxAPIResponseBytes,
	}
	return response, nil
}

// ApiClient provides an uncapped API client used for ordinary REANA operations.
func ApiClient() (*API, error) {
	return newAPIClient(0, false)
}

// ControlAPIClient provides a bounded client for small control-plane responses.
func ControlAPIClient() (*API, error) {
	return newAPIClient(apiRequestTimeout, true)
}

// StreamingHTTPClient returns a client for large raw request bodies and bounded
// control-plane responses. It has no whole-request deadline, but bounds the
// wait for response headers after the body has been transmitted.
func StreamingHTTPClient() (*http.Client, *url.URL, error) {
	serverURL := viper.GetString("server-url")
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, nil, err
	}
	if u.Host == "" {
		return nil, nil, errors.New(
			"environment variable REANA_SERVER_URL is not set",
		)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, nil, fmt.Errorf(
			"unsupported REANA server URL scheme %q",
			u.Scheme,
		)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{ //nolint:gosec
		InsecureSkipVerify: true,
	}
	transport.ResponseHeaderTimeout = apiRequestTimeout
	return &http.Client{
		Transport: &boundedResponseTransport{transport: transport},
	}, u, nil
}

func newAPIClient(
	requestTimeout time.Duration,
	boundResponses bool,
) (*API, error) {
	// parse REANA server URL
	serverURL := viper.GetString("server-url")
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, errors.New(
			"environment variable REANA_SERVER_URL is not set",
		)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf(
			"unsupported REANA server URL scheme %q",
			u.Scheme,
		)
	}

	// Keep the historical support for self-signed cluster certificates without
	// mutating the process-wide default HTTP transport.
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = &tls.Config{ //nolint:gosec
		InsecureSkipVerify: true,
	}
	var transport http.RoundTripper = baseTransport
	if boundResponses {
		transport = &boundedResponseTransport{
			transport: &knownLengthMultipartTransport{transport: baseTransport},
		}
	}
	httpClient := &http.Client{
		Timeout:   requestTimeout,
		Transport: transport,
	}
	apiTransport := httptransport.NewWithClient(
		u.Host,
		"",
		[]string{u.Scheme},
		httpClient,
	)
	apiTransport.SetLogger(log.StandardLogger())
	apiTransport.SetDebug(log.GetLevel() == log.DebugLevel)
	apiTransport.Consumers["application/zip"] = runtime.ByteStreamConsumer()

	log.Info("Connecting to ", serverURL)

	// create the API client, with the transport
	return New(apiTransport, strfmt.Default), nil
}
