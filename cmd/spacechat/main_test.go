package main

import (
	"errors"
	"testing"

	"xchat/internal/launcher"
	"xchat/internal/update"
)

type exitError int

func (failure exitError) Error() string { return "child failed" }
func (failure exitError) ExitCode() int { return int(failure) }

func TestRunForwardsChildExitCode(t *testing.T) {
	resolve := func() (update.Layout, error) { return update.Layout{}, nil }
	runner := func(update.Layout, string, []string, launcher.Streams, launcher.CommandRunner) error {
		return exitError(7)
	}
	if code := run(nil, launcher.Streams{}, resolve, runner, nil); code != 7 {
		t.Fatalf("run exit code = %d", code)
	}
}

func TestRunReportsSetupFailure(t *testing.T) {
	resolve := func() (update.Layout, error) { return update.Layout{}, errors.New("no layout") }
	if code := run(nil, launcher.Streams{}, resolve, nil, nil); code != 1 {
		t.Fatalf("run exit code = %d", code)
	}
}
