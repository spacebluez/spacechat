package main

import (
	"fmt"
	"os"
	"runtime"

	"xchat/internal/launcher"
	"xchat/internal/update"
)

type layoutResolver func() (update.Layout, error)
type launcherRunner func(update.Layout, string, []string, launcher.Streams, launcher.CommandRunner) error

func resolveLayout() (update.Layout, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return update.Layout{}, err
	}
	return update.LayoutFor(runtime.GOOS, home, os.Getenv)
}

func run(args []string, streams launcher.Streams, resolve layoutResolver, launch launcherRunner, command launcher.CommandRunner) int {
	if resolve == nil {
		resolve = resolveLayout
	}
	layout, err := resolve()
	if err == nil {
		if launch == nil {
			launch = launcher.Run
		}
		err = launch(layout, runtime.GOOS, args, streams, command)
	}
	if err == nil {
		return 0
	}
	if streams.Stderr != nil {
		fmt.Fprintln(streams.Stderr, "spacechat:", err)
	}
	if failure, ok := err.(interface{ ExitCode() int }); ok && failure.ExitCode() >= 0 {
		return failure.ExitCode()
	}
	return 1
}

func main() {
	streams := launcher.Streams{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	os.Exit(run(os.Args[1:], streams, nil, nil, nil))
}
