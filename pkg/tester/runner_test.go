/*
This file is part of REANA.
Copyright (C) 2026 CERN.

REANA is free software; you can redistribute it and/or modify it
under the terms of the MIT License; see LICENSE file for more details.
*/

package tester

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type mockFetcher struct {
	workflowStatus WorkflowStatus
	logs           WorkflowLogs
	logSteps       [][]string
}

func newMockFetcher() *mockFetcher {
	return &mockFetcher{
		workflowStatus: WorkflowStatus{
			Name:          "analysis.1",
			Status:        "finished",
			RunStartedAt:  "2026-08-23T09:00:00",
			RunFinishedAt: "2026-08-23T09:05:00",
		},
		logs: WorkflowLogs{
			WorkflowLogs:   "Building DAG of jobs\n3 of 3 steps (100%) done",
			EngineSpecific: json.RawMessage(`"engine-specific output"`),
			JobLogs: map[string]JobLog{
				"1": {
					JobName: "gendata",
					Logs: "variables\n---------\n" +
						"(a0,a1,mean,nbkg,nsig,sig1frac,sigma1,x)\n" +
						"datasets\n--------\nRooDataSet::modelData(x)",
					StartedAt:  "2026-08-23T09:00:00",
					FinishedAt: "2026-08-23T09:02:00",
				},
				"2": {
					JobName:    "fitdata",
					Logs:       "MIGRAD MINIMIZATION HAS CONVERGED.",
					StartedAt:  "2026-08-23T09:02:00",
					FinishedAt: "2026-08-23T09:04:00",
				},
			},
		},
	}
}

func (f *mockFetcher) Status(string) (WorkflowStatus, error) {
	return f.workflowStatus, nil
}

func (f *mockFetcher) Specification(string) (WorkflowSpecification, error) {
	return WorkflowSpecification{
		OutputFiles:       []string{"results/message.txt"},
		OutputDirectories: []string{"results"},
		TestFiles:         []string{"tests/demo.feature"},
	}, nil
}

func (f *mockFetcher) Files(_, fileName string) ([]FileInfo, error) {
	if fileName == "results/data.root" {
		return []FileInfo{{
			Name:          fileName,
			Size:          155 * 1024,
			HumanReadable: "155 KiB",
		}}, nil
	}
	return nil, nil
}

func (f *mockFetcher) DiskUsage(
	_ string,
	summarize bool,
) ([]FileInfo, error) {
	if summarize {
		return []FileInfo{{Size: 160 * 1024}}, nil
	}
	return []FileInfo{
		{Name: "/code/gendata.C"},
		{Name: "/results"},
		{Name: "/results/message.txt"},
	}, nil
}

func (f *mockFetcher) Logs(_ string, steps []string) (WorkflowLogs, error) {
	f.logSteps = append(f.logSteps, append([]string{}, steps...))
	if len(steps) == 0 {
		return f.logs, nil
	}
	filtered := WorkflowLogs{JobLogs: map[string]JobLog{}}
	for id, job := range f.logs.JobLogs {
		if job.JobName == steps[0] {
			filtered.JobLogs[id] = job
		}
	}
	return filtered, nil
}

func (f *mockFetcher) Download(_, fileName string) (DownloadedFile, error) {
	if strings.TrimPrefix(fileName, "/") == "results/message.txt" {
		return DownloadedFile{Content: []byte("hello world")}, nil
	}
	return DownloadedFile{}, errors.New("file not found")
}

func writeFeature(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.feature")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseAndRunDemoVocabulary(t *testing.T) {
	fetcher := newMockFetcher()
	feature, results, err := ParseAndRun(
		"testdata/demo.feature",
		"analysis.1",
		fetcher,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if feature != "ROOT6 workflow checks" {
		t.Errorf("unexpected feature name %q", feature)
	}
	if len(results) != 11 {
		t.Fatalf("got %d results, want 11", len(results))
	}
	for _, result := range results {
		if result.Status != Passed {
			t.Errorf(
				"scenario %q returned %s: %s",
				result.Scenario,
				result.Status,
				result.ErrorLog,
			)
		}
	}
	wantFilteredCalls := [][]string{
		{"gendata"},
		{"gendata"},
		{"fitdata"},
		{"gendata"},
	}
	var filteredCalls [][]string
	for _, steps := range fetcher.logSteps {
		if len(steps) > 0 {
			filteredCalls = append(filteredCalls, steps)
		}
	}
	if !reflect.DeepEqual(filteredCalls, wantFilteredCalls) {
		t.Errorf(
			"unexpected filtered log calls: got %v, want %v",
			filteredCalls,
			wantFilteredCalls,
		)
	}
}

func TestParseAndRunRecordsFailedScenarioAndContinues(t *testing.T) {
	feature := writeFeature(t, `Feature: failures
Scenario: fails
  When the workflow is finished
  Then the engine logs should contain "absent"
Scenario: passes
  When the workflow is finished
  Then the engine logs should contain "Building DAG"
`)
	_, results, err := ParseAndRun(feature, "analysis.1", newMockFetcher())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Status != Failed ||
		!strings.Contains(results[0].FailedTestcase, "absent") {
		t.Errorf("unexpected failed result: %+v", results[0])
	}
	if results[1].Status != Passed {
		t.Errorf("runner did not continue after failure: %+v", results[1])
	}
}

func TestParseAndRunRecordsSkippedScenario(t *testing.T) {
	feature := writeFeature(t, `Feature: skipped
Scenario: waits for completion
  When the workflow execution completes
  Then the workflow status should be finished
`)
	fetcher := newMockFetcher()
	fetcher.workflowStatus.Status = "running"
	_, results, err := ParseAndRun(feature, "analysis.1", fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Status != Skipped {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestParseAndRunRejectsUnknownStepBeforeExecution(t *testing.T) {
	feature := writeFeature(t, `Feature: unsupported
Scenario: unsupported
  When an unknown action happens
`)
	_, _, err := ParseAndRun(feature, "analysis.1", newMockFetcher())
	var missing *StepDefinitionNotFound
	if !errors.As(err, &missing) {
		t.Fatalf("expected StepDefinitionNotFound, got %v", err)
	}
}

func TestParseAndRunErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, _, err := ParseAndRun(
			"missing.feature",
			"analysis.1",
			newMockFetcher(),
		)
		if !os.IsNotExist(err) {
			t.Fatalf("expected file-not-found error, got %v", err)
		}
	})

	t.Run("malformed feature", func(t *testing.T) {
		feature := writeFeature(
			t,
			"Feature: malformed\n Scenario: bad\n  Given a value\n   \"\"\"\n   unclosed",
		)
		_, _, err := ParseAndRun(feature, "analysis.1", newMockFetcher())
		var featureError *FeatureFileError
		if !errors.As(err, &featureError) {
			t.Fatalf("expected FeatureFileError, got %v", err)
		}
		if !strings.Contains(featureError.Error(), "unexpected error") {
			t.Errorf("unexpected feature error: %v", featureError)
		}
		if featureError.Unwrap() == nil {
			t.Error("feature error did not retain its cause")
		}
	})
}

func TestHumanReadableToBytes(t *testing.T) {
	tests := map[string]int64{
		"12345":    12345,
		"5656 B":   5656,
		"89 bytes": 89,
		"1.0KiB":   1024,
		"3.2 GiB":  3435973836,
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			actual, err := humanReadableToBytes(input)
			if err != nil {
				t.Fatal(err)
			}
			if actual != expected {
				t.Errorf("got %d, want %d", actual, expected)
			}
		})
	}
}

func TestFindJobSelectsLatestMatchingEntryDeterministically(t *testing.T) {
	tests := map[string]struct {
		logs WorkflowLogs
		want string
	}{
		"latest start time takes precedence over job ID": {
			logs: WorkflowLogs{JobLogs: map[string]JobLog{
				"3": {JobName: "other", Logs: "other"},
				"z": {
					JobName:   "step",
					Logs:      "first",
					StartedAt: "2026-08-23T09:00:00",
				},
				"a": {
					JobName:   "step",
					Logs:      "last",
					StartedAt: "2026-08-23T09:01:00",
				},
			}},
			want: "last",
		},
		"job ID breaks start time ties": {
			logs: WorkflowLogs{JobLogs: map[string]JobLog{
				"a": {
					JobName:   "step",
					Logs:      "first",
					StartedAt: "2026-08-23T09:00:00",
				},
				"z": {
					JobName:   "step",
					Logs:      "last",
					StartedAt: "2026-08-23T09:00:00",
				},
			}},
			want: "last",
		},
		"missing start times fall back to job ID": {
			logs: WorkflowLogs{JobLogs: map[string]JobLog{
				"a": {JobName: "step", Logs: "first"},
				"z": {JobName: "step", Logs: "last"},
			}},
			want: "last",
		},
		"started job wins over job with missing start time": {
			logs: WorkflowLogs{JobLogs: map[string]JobLog{
				"z": {JobName: "step", Logs: "not started"},
				"a": {
					JobName:   "step",
					Logs:      "started",
					StartedAt: "2026-08-23T09:00:00",
				},
			}},
			want: "started",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			job, err := findJob(test.logs, "step")
			if err != nil {
				t.Fatal(err)
			}
			if job.Logs != test.want {
				t.Errorf("got logs %q, want %q", job.Logs, test.want)
			}
		})
	}
}

func TestFindJobRejectsUnknownStepName(t *testing.T) {
	if _, err := findJob(
		WorkflowLogs{JobLogs: map[string]JobLog{
			"1": {JobName: "other"},
		}},
		"step",
	); err == nil {
		t.Error("expected an invalid step name error")
	}
}

func TestStepDefinitionNotFoundMessage(t *testing.T) {
	err := (&StepDefinitionNotFound{Step: "unsupported"}).Error()
	if !strings.Contains(err, "unsupported") {
		t.Errorf("unexpected missing step error: %s", err)
	}
}

func TestFileChecksumAlgorithms(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
		checksum  string
		wantError bool
	}{
		{
			name:      "sha512",
			algorithm: "sha512",
			checksum:  "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f",
		},
		{
			name:      "md5",
			algorithm: "md5",
			checksum:  "5eb63bbbe01eeed093cb22bb8f5acdc3",
		},
		{
			name:      "adler32",
			algorithm: "adler32",
			checksum:  "0x1a0b045d",
		},
		{
			name:      "unsupported",
			algorithm: "crc32",
			checksum:  "unused",
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := assertFileChecksum(newMockFetcher())(
				"analysis.1",
				map[string]string{
					"algorithm": test.algorithm,
					"filename":  `"results/message.txt"`,
					"checksum":  test.checksum,
				},
			)
			if test.wantError && err == nil {
				t.Fatal("expected checksum error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("unexpected checksum error: %v", err)
			}
		})
	}
}
