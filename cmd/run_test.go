/*
This file is part of REANA.
Copyright (C) 2026 CERN.

REANA is free software; you can redistribute it and/or modify it
under the terms of the MIT License; see LICENSE file for more details.
*/

package cmd

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestRunCreatesUploadsAndStartsWorkflow(t *testing.T) {
	reanaFile := writeSerialSpec(t)
	var requests []string

	server := httptest.NewTLSServer(
		withPingFunc(func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/api/workflows":
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write(
					[]byte(`{"workflow_name":"analysis.1","workflow_id":"id"}`),
				)
			case r.Method == http.MethodGet && r.URL.Path == "/api/workflows/analysis.1/specification":
				_, _ = w.Write(
					[]byte(
						`{"parameters":{},"specification":{"inputs":{"files":[],"directories":[]},"workflow":{"type":"serial","specification":{"steps":[]}}}}`,
					),
				)
			case r.Method == http.MethodPost && r.URL.Path == "/api/workflows/analysis.1/start":
				_, _ = w.Write(
					[]byte(
						`{"message":"started","status":"running","user":"u","workflow_id":"id","workflow_name":"analysis.1"}`,
					),
				)
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}),
	)
	defer server.Close()

	viper.Set("server-url", server.URL)
	t.Cleanup(viper.Reset)
	out, err := ExecuteCommand(
		NewRootCmd(),
		"run",
		"-t", "1234",
		"-f", reanaFile,
		"-w", "analysis",
	)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	wantRequests := []string{
		"POST /api/workflows",
		"GET /api/workflows/analysis.1/specification",
		"POST /api/workflows/analysis.1/start",
	}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Fatalf(
			"unexpected request sequence: got %v, want %v",
			requests,
			wantRequests,
		)
	}
	for _, message := range []string{
		"Creating a workflow...",
		"analysis.1",
		"Uploading files...",
		"Starting workflow...",
		"analysis.1 is running",
	} {
		if !strings.Contains(out, message) {
			t.Errorf("expected %q in output, got %q", message, out)
		}
	}
}

func TestRunStopsWhenCreationFails(t *testing.T) {
	reanaFile := writeSerialSpec(t)
	requestCount := 0
	server := httptest.NewTLSServer(
		withPingFunc(func(w http.ResponseWriter, _ *http.Request) {
			requestCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"invalid workflow"}`))
		}),
	)
	defer server.Close()

	viper.Set("server-url", server.URL)
	t.Cleanup(viper.Reset)
	_, err := ExecuteCommand(NewRootCmd(), "run", "-t", "1234", "-f", reanaFile)
	if err == nil || !strings.Contains(err.Error(), "invalid workflow") {
		t.Fatalf("expected the creation error, got %v", err)
	}
	if requestCount != 1 {
		t.Fatalf(
			"run continued after creation failure: got %d requests",
			requestCount,
		)
	}
}
