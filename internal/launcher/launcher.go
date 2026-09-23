package launcher

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"xchat/internal/uninstall"
	"xchat/internal/update"
)

type Streams struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

type CommandRunner func(path string, args []string, streams Streams) error

func Run(layout update.Layout, goos string, args []string, streams Streams, runner CommandRunner) error {
	if len(args) > 0 && args[0] == "uninstall" {
		return uninstall.Run(layout, args[1:], streams.Stdout)
	}
	version, err := update.ReadCurrent(layout)
	if err != nil {
		return fmt.Errorf("read active SpaceChat version: %w", err)
	}
	client, err := layout.ClientPath(version, goos)
	if err != nil {
		return err
	}
	info, err := os.Lstat(client)
	if err != nil {
		return fmt.Errorf("find active SpaceChat client: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("active SpaceChat client must be a regular managed file")
	}
	if runner == nil {
		runner = Command
	}
	return runner(client, args, streams)
}

func Command(path string, args []string, streams Streams) error {
	command := exec.Command(path, args...)
	command.Dir = filepath.Dir(path)
	command.Stdin = streams.Stdin
	command.Stdout = streams.Stdout
	command.Stderr = streams.Stderr
	return command.Run()
}
