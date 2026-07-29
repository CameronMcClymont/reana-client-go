/*
This file is part of REANA.
Copyright (C) 2026 CERN.

REANA is free software; you can redistribute it and/or modify it
under the terms of the MIT License; see LICENSE file for more details.
*/

package cmd

import (
	"net/http"
)

// capablePingBody is what a server implementing the specification-bundle
// protocol answers on the unauthenticated /api/ping.
const capablePingBody = `{"message":"OK","status":"200",` +
	`"reana_server_version":"0.95.0a6",` +
	`"api_capabilities":["workflow-specification-bundles-v1"]}`

// legacyPingBody is what a released server answers: no capability list at all.
const legacyPingBody = `{"message":"OK","status":"200"}`

// withPing serves the capability preflight separately from the operation under
// test, so a test server answering a single path does not accidentally reply to
// /api/ping with that operation's status code.
//
// Pass a body (e.g. legacyPingBody) to simulate a server that does not
// advertise the protocol; omit it for a capable server.
func withPing(handler http.Handler, body ...string) http.Handler {
	pingBody := capablePingBody
	if len(body) > 0 {
		pingBody = body[0]
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ping" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(pingBody))
			return
		}
		handler.ServeHTTP(w, r)
	})
}

// withPingFunc is withPing for a bare handler function.
func withPingFunc(
	handler func(w http.ResponseWriter, r *http.Request),
	body ...string,
) http.Handler {
	return withPing(http.HandlerFunc(handler), body...)
}
