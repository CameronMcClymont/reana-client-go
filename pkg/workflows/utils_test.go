/*
This file is part of REANA.
Copyright (C) 2022, 2024, 2025, 2026 CERN.

REANA is free software; you can redistribute it and/or modify it
under the terms of the MIT License; see LICENSE file for more details.
*/

package workflows

import (
	"testing"
)

func TestGetNameAndRunNumber(t *testing.T) {
	tests := map[string]struct {
		arg          string
		workflowName string
		runNumber    string
	}{
		"only name": {
			arg:          "foo",
			workflowName: "foo",
			runNumber:    "",
		},
		"name and run number": {
			arg:          "foo.bar",
			workflowName: "foo",
			runNumber:    "bar",
		},
		"run number with dots": {
			arg:          "foo.bar.baz",
			workflowName: "foo",
			runNumber:    "bar.baz",
		},
		"empty string": {arg: "", workflowName: "", runNumber: ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			workflowName, runNumber := GetNameAndRunNumber(test.arg)
			if workflowName != test.workflowName {
				t.Errorf("Expected %s, got %s", test.workflowName, workflowName)
			}
			if runNumber != test.runNumber {
				t.Errorf("Expected %s, got %s", test.runNumber, runNumber)
			}
		})
	}
}

func TestParseWorkflowRunNumber(t *testing.T) {
	tests := map[string]struct {
		fullName string
		baseName string
		major    string
		minor    string
	}{
		"empty": {},
		"workflow name": {
			fullName: "analysis",
			baseName: "analysis",
		},
		"major run number": {
			fullName: "analysis.7",
			baseName: "analysis",
			major:    "7",
		},
		"minor run number": {
			fullName: "analysis.7.1",
			baseName: "analysis",
			major:    "7",
			minor:    "1",
		},
		"nested minor run number": {
			fullName: "analysis.7.1.2",
			baseName: "analysis",
			major:    "7",
			minor:    "1.2",
		},
		"dotted workflow name": {
			fullName: "physics.analysis.7.1",
			baseName: "physics.analysis",
			major:    "7",
			minor:    "1",
		},
		"numeric name": {
			fullName: "7.1",
			major:    "7",
			minor:    "1",
		},
		"uuid": {
			fullName: "86f28b84-d59d-43ed-a8dd-7b4dada3aaa0",
			baseName: "86f28b84-d59d-43ed-a8dd-7b4dada3aaa0",
		},
		"non-numeric suffix": {
			fullName: "analysis.alpha.2",
			baseName: "analysis.alpha",
			major:    "2",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			baseName, major, minor := ParseWorkflowRunNumber(test.fullName)
			if baseName != test.baseName || major != test.major ||
				minor != test.minor {
				t.Errorf(
					"ParseWorkflowRunNumber(%q) = (%q, %q, %q), want (%q, %q, %q)",
					test.fullName,
					baseName,
					major,
					minor,
					test.baseName,
					test.major,
					test.minor,
				)
			}
		})
	}
}

func TestGetRunNumberMajorKey(t *testing.T) {
	tests := map[string]struct {
		fullName string
		want     string
	}{
		"no run number": {fullName: "analysis"},
		"major run": {
			fullName: "analysis.7",
			want:     "analysis.7",
		},
		"restart": {
			fullName: "analysis.7.2",
			want:     "analysis.7",
		},
		"dotted workflow name": {
			fullName: "physics.analysis.7.2",
			want:     "physics.analysis.7",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := GetRunNumberMajorKey(test.fullName); got != test.want {
				t.Errorf(
					"GetRunNumberMajorKey(%q) = %q, want %q",
					test.fullName,
					got,
					test.want,
				)
			}
		})
	}
}

func TestFormatRunNumberLabel(t *testing.T) {
	tests := map[string]struct {
		fullName string
		want     string
	}{
		"no run number": {
			fullName: "analysis",
			want:     "analysis",
		},
		"major run": {
			fullName: "analysis.7",
			want:     "#7",
		},
		"restart": {
			fullName: "analysis.7.2",
			want:     "#7.2",
		},
		"nested restart": {
			fullName: "analysis.7.2.1",
			want:     "#7.2.1",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := FormatRunNumberLabel(test.fullName); got != test.want {
				t.Errorf(
					"FormatRunNumberLabel(%q) = %q, want %q",
					test.fullName,
					got,
					test.want,
				)
			}
		})
	}
}

func TestFormatRunLabelList(t *testing.T) {
	tests := map[string]struct {
		labels    []string
		maxLabels int
		want      string
	}{
		"empty": {},
		"short list": {
			labels:    []string{"#7", "#7.1"},
			maxLabels: 10,
			want:      "#7, #7.1",
		},
		"empty labels": {
			labels:    []string{"#7", "", "#7.1"},
			maxLabels: 10,
			want:      "#7, #7.1",
		},
		"truncated list": {
			labels:    []string{"#7", "#7.1", "#7.2", "#7.3"},
			maxLabels: 2,
			want:      "#7, #7.1, +2 more",
		},
		"unlimited list": {
			labels:    []string{"#7", "#7.1", "#7.2"},
			maxLabels: 0,
			want:      "#7, #7.1, #7.2",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := FormatRunLabelList(test.labels, test.maxLabels); got != test.want {
				t.Errorf(
					"FormatRunLabelList(%v, %d) = %q, want %q",
					test.labels,
					test.maxLabels,
					got,
					test.want,
				)
			}
		})
	}
}

func TestGetDuration(t *testing.T) {
	curTime := "2020-01-01T03:16:45"
	future := "2020-01-01T03:16:46"
	past := "2020-01-01T03:16:44"
	badFormat := "not_a_date"

	tests := map[string]struct {
		runStartedAt  *string
		runFinishedAt *string
		runStoppedAt  *string
		want          any
		wantError     bool
	}{
		"finished instantly": {
			runStartedAt:  &curTime,
			runFinishedAt: &curTime,
			want:          0.0,
		},
		"finished in 1 second": {
			runStartedAt:  &curTime,
			runFinishedAt: &future,
			want:          1.0,
		},
		"finished before start": {
			runStartedAt:  &curTime,
			runFinishedAt: &past,
			want:          -1.0,
		},
		"stopped in 1 second": {
			runStartedAt:  &curTime,
			runFinishedAt: nil,
			runStoppedAt:  &future,
			want:          1.0,
		},
		"nil arguments": {
			runStartedAt:  nil,
			runFinishedAt: nil,
			runStoppedAt:  nil,
			want:          nil,
		},
		"nil start": {
			runStartedAt:  nil,
			runFinishedAt: &curTime,
			want:          nil,
		},
		"nil finish":       {runStartedAt: &curTime, runFinishedAt: nil},
		"bad start format": {runStartedAt: &badFormat, wantError: true},
		"bad stop format": {
			runStartedAt: &curTime,
			runStoppedAt: &badFormat,
			wantError:    true,
		},
		"bad finish format": {
			runStartedAt:  &curTime,
			runFinishedAt: &badFormat,
			wantError:     true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := GetDuration(
				test.runStartedAt,
				test.runFinishedAt,
				test.runStoppedAt,
			)
			if test.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if test.runStartedAt != nil && test.runFinishedAt == nil {
					duration, ok := got.(float64)
					if !ok || duration <= 0 {
						t.Errorf("Expected positive duration, got %v", got)
					}
				} else if got != test.want {
					t.Errorf("Expected %v, got %v", test.want, got)
				}
			}
		})
	}
}

func TestStatusChangeMessage(t *testing.T) {
	tests := map[string]struct {
		workflow  string
		status    string
		expected  string
		wantError bool
	}{
		"running": {
			workflow: "workflow",
			status:   "running",
			expected: "workflow is running",
		},
		"pending": {
			workflow: "workflow",
			status:   "pending",
			expected: "workflow is pending",
		},
		"deleted": {
			workflow: "workflow",
			status:   "deleted",
			expected: "workflow has been deleted",
		},
		"created": {
			workflow: "workflow",
			status:   "created",
			expected: "workflow has been created",
		},
		"stopped": {
			workflow: "workflow",
			status:   "stopped",
			expected: "workflow has been stopped",
		},
		"queued": {
			workflow: "workflow",
			status:   "queued",
			expected: "workflow has been queued",
		},
		"finished": {
			workflow: "workflow",
			status:   "finished",
			expected: "workflow has finished",
		},
		"failed": {
			workflow: "workflow",
			status:   "failed",
			expected: "workflow has failed",
		},
		"invalid status": {
			workflow:  "workflow",
			status:    "invalid",
			expected:  "unrecognised status invalid",
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := StatusChangeMessage(test.workflow, test.status)
			if err == nil {
				if test.wantError {
					t.Errorf("Expected error, got nil")
				} else if got != test.expected {
					t.Errorf("Expected %s, got %s", test.expected, got)
				}
			} else {
				if !test.wantError {
					t.Errorf("Expected no error, got %s", err.Error())
				} else if err.Error() != test.expected {
					t.Errorf("Expected %s error, got %s", test.expected, err.Error())
				}
			}
		})
	}
}
