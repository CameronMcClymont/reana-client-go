/*
This file is part of REANA.
Copyright (C) 2026 CERN.

REANA is free software; you can redistribute it and/or modify it
under the terms of the MIT License; see LICENSE file for more details.
*/

// Package tester evaluates Gherkin assertions against a REANA workflow run.
package tester

import (
	"encoding/json"
	"time"
)

// Status describes the outcome of a workflow test scenario.
type Status string

const (
	// Passed indicates that every scenario step completed successfully.
	Passed Status = "passed"
	// Failed indicates that a scenario assertion failed.
	Failed Status = "failed"
	// Skipped indicates that a scenario precondition was not met.
	Skipped Status = "skipped"
)

// Result contains the outcome of one Gherkin scenario.
type Result struct {
	Scenario       string
	FailedTestcase string
	Status         Status
	ErrorLog       string
	Feature        string
	CheckedAt      time.Time
}

// WorkflowStatus contains workflow state used by test assertions.
type WorkflowStatus struct {
	Name          string
	Status        string
	RunStartedAt  string
	RunFinishedAt string
}

// WorkflowSpecification contains specification data used by test assertions.
type WorkflowSpecification struct {
	OutputFiles       []string
	OutputDirectories []string
	TestFiles         []string
}

// FileInfo describes one workflow workspace entry.
type FileInfo struct {
	Name          string
	Size          int64
	HumanReadable string
}

// JobLog contains the log output and execution timestamps for one job.
type JobLog struct {
	JobName    string `json:"job_name"`
	Logs       string `json:"logs"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// WorkflowLogs contains engine and job logs returned by REANA.
type WorkflowLogs struct {
	WorkflowLogs string            `json:"workflow_logs"`
	JobLogs      map[string]JobLog `json:"job_logs"`
	// EngineSpecific is raw because engines return text or structured data.
	// Decoding into a string would prevent job-log assertions from running.
	EngineSpecific json.RawMessage `json:"engine_specific"`
}

// DownloadedFile contains raw workspace file data.
type DownloadedFile struct {
	Content   []byte
	IsArchive bool
}

// DataFetcher retrieves workflow data needed by the step definitions.
type DataFetcher interface {
	Status(workflow string) (WorkflowStatus, error)
	Specification(workflow string) (WorkflowSpecification, error)
	Files(workflow, fileName string) ([]FileInfo, error)
	DiskUsage(workflow string, summarize bool) ([]FileInfo, error)
	Logs(workflow string, steps []string) (WorkflowLogs, error)
	Download(workflow, fileName string) (DownloadedFile, error)
}
