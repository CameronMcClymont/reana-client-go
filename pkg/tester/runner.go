/*
This file is part of REANA.
Copyright (C) 2026 CERN.

REANA is free software; you can redistribute it and/or modify it
under the terms of the MIT License; see LICENSE file for more details.
*/

package tester

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	gherkin "github.com/cucumber/gherkin/go/v42"
	messages "github.com/cucumber/messages/go/v34"
)

// FeatureFileError identifies malformed Gherkin input.
type FeatureFileError struct {
	Path string
	Err  error
}

func (e *FeatureFileError) Error() string {
	return fmt.Sprintf(
		"unexpected error during parsing or compiling of the test file %q: %v",
		e.Path,
		e.Err,
	)
}

func (e *FeatureFileError) Unwrap() error {
	return e.Err
}

// StepDefinitionNotFound identifies an unsupported Gherkin step.
type StepDefinitionNotFound struct {
	Step string
}

func (e *StepDefinitionNotFound) Error() string {
	return fmt.Sprintf("no step definition found for step: %s", e.Step)
}

type stepSkipped struct {
	reason string
}

func (e *stepSkipped) Error() string {
	return e.reason
}

type stepHandler func(workflow string, arguments map[string]string) error

type stepDefinition struct {
	stepType messages.PickleStepType
	pattern  *regexp.Regexp
	handler  stepHandler
}

type matchedStep struct {
	definition stepDefinition
	arguments  map[string]string
}

var placeholderPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func newStepDefinition(
	stepType messages.PickleStepType,
	pattern string,
	handler stepHandler,
) stepDefinition {
	var expression strings.Builder
	expression.WriteString("^")
	last := 0
	for _, location := range placeholderPattern.FindAllStringSubmatchIndex(
		pattern,
		-1,
	) {
		expression.WriteString(regexp.QuoteMeta(pattern[last:location[0]]))
		expression.WriteString("(?P<")
		expression.WriteString(pattern[location[2]:location[3]])
		expression.WriteString(">(?s:.+?))")
		last = location[1]
	}
	expression.WriteString(regexp.QuoteMeta(pattern[last:]))
	expression.WriteString("$")
	return stepDefinition{
		stepType: stepType,
		pattern:  regexp.MustCompile(expression.String()),
		handler:  handler,
	}
}

func stepText(step *messages.PickleStep) string {
	text := step.Text
	if step.Argument != nil && step.Argument.DocString != nil {
		text += ` "` + step.Argument.DocString.Content + `"`
	}
	return text
}

func matchStep(
	step *messages.PickleStep,
	definitions []stepDefinition,
) (matchedStep, error) {
	text := stepText(step)
	for _, definition := range definitions {
		if definition.stepType != step.Type {
			continue
		}
		matches := definition.pattern.FindStringSubmatch(text)
		if matches == nil {
			continue
		}
		arguments := make(map[string]string)
		for index, name := range definition.pattern.SubexpNames() {
			if index > 0 && name != "" {
				arguments[name] = matches[index]
			}
		}
		return matchedStep{
			definition: definition,
			arguments:  arguments,
		}, nil
	}
	return matchedStep{}, &StepDefinitionNotFound{Step: text}
}

// ParseAndRun parses a Gherkin file and evaluates each scenario in order.
func ParseAndRun(
	featureFilePath,
	workflow string,
	dataFetcher DataFetcher,
) (string, []Result, error) {
	featureFile, err := os.Open(featureFilePath)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = featureFile.Close() }()

	idGenerator := &messages.Incrementing{}
	document, err := gherkin.ParseGherkinDocument(
		featureFile,
		idGenerator.NewId,
	)
	if err != nil {
		return "", nil, &FeatureFileError{Path: featureFilePath, Err: err}
	}
	if document.Feature == nil {
		return "", nil, &FeatureFileError{
			Path: featureFilePath,
			Err:  errors.New("feature is missing"),
		}
	}

	pickles := gherkin.Pickles(
		*document,
		featureFilePath,
		idGenerator.NewId,
	)
	definitions := stepDefinitions(dataFetcher)
	matches := make(map[*messages.PickleStep]matchedStep)
	for _, pickle := range pickles {
		for _, step := range pickle.Steps {
			matched, err := matchStep(step, definitions)
			if err != nil {
				return "", nil, err
			}
			matches[step] = matched
		}
	}

	featureName := document.Feature.Name
	results := make([]Result, 0, len(pickles))
	for _, pickle := range pickles {
		result := Result{
			Scenario:  pickle.Name,
			Status:    Passed,
			Feature:   featureName,
			CheckedAt: time.Now().UTC(),
		}
		for _, step := range pickle.Steps {
			matched := matches[step]
			err := matched.definition.handler(workflow, matched.arguments)
			if err == nil {
				continue
			}
			var skipped *stepSkipped
			if errors.As(err, &skipped) {
				result.Status = Skipped
				result.ErrorLog = skipped.Error()
				break
			}
			result.Status = Failed
			result.FailedTestcase = stepText(step)
			result.ErrorLog = err.Error()
			break
		}
		results = append(results, result)
	}
	return featureName, results, nil
}
