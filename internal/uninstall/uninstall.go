package uninstall

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"xchat/internal/update"
)

type plan struct {
	layout   update.Layout
	home     string
	launcher string
	purge    bool
}

func Run(layout update.Layout, args []string, output io.Writer) error {
	if output == nil {
		output = io.Discard
	}
	flags := flag.NewFlagSet("spacechat uninstall", flag.ContinueOnError)
	flags.SetOutput(output)
	purge := flags.Bool("purge", false, "also remove the saved connection configuration")
	flags.Usage = func() {
		fmt.Fprintln(output, "Usage: spacechat uninstall [--purge]")
		fmt.Fprintln(output, "Removes this user's client, installed versions, and bundled terminal. Configuration is kept unless --purge is used.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: spacechat uninstall [--purge]")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	expected, err := update.LayoutFor(runtime.GOOS, home, os.Getenv)
	if err != nil {
		return err
	}
	if layout != expected {
		return errors.New("refusing to uninstall outside the current user's installation")
	}
	name := "spacechat"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := plan{layout: layout, home: home, launcher: filepath.Join(layout.Bin, name), purge: *purge}
	if err := p.validate(); err != nil {
		return err
	}
	if err := checkRunning(p); err != nil {
		return err
	}
	var lock *update.Lock
	if _, err := os.Stat(layout.Root); err == nil {
		lock, err = update.Acquire(layout, time.Now())
		if err != nil {
			return err
		}
		defer lock.Release()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, path := range p.directories() {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove %s (close any running SpaceChat windows and retry): %w", path, err)
		}
	}
	files := []string{layout.Current}
	if p.purge {
		files = append(files, layout.Config)
	}
	for _, path := range files {
		if err := removeFile(path); err != nil {
			return err
		}
	}
	if err := cleanPath(p); err != nil {
		return fmt.Errorf("remove SpaceChat from PATH: %w", err)
	}
	pending, err := removeLauncher(p)
	if err != nil {
		return err
	}
	if err := lock.Release(); err != nil {
		return err
	}
	for _, path := range []string{filepath.Dir(layout.Config), layout.Root} {
		if err := removeEmptyDirectory(path); err != nil {
			return err
		}
	}
	if pending {
		fmt.Fprintln(output, "SpaceChat removed. Launcher cleanup will finish after this command exits.")
	} else {
		fmt.Fprintln(output, "SpaceChat uninstalled.")
	}
	if !p.purge {
		if _, err := os.Stat(layout.Config); err == nil {
			fmt.Fprintln(output, "Configuration kept at:", layout.Config)
		}
	}
	fmt.Fprintln(output, "Open a new terminal to refresh command lookup.")
	return nil
}

func (p plan) directories() []string {
	paths := []string{p.layout.Versions, p.layout.Temp}
	if runtime.GOOS == "windows" {
		paths = append(paths, filepath.Join(p.layout.Root, "terminal"))
	}
	return paths
}

func (p plan) validate() error {
	paths := append(p.directories(), p.layout.Root, p.layout.Bin, filepath.Dir(p.layout.Config), p.home)
	for _, path := range paths {
		if err := safePath(path, true); err != nil {
			return err
		}
	}
	for _, path := range []string{p.launcher, p.layout.Current, p.layout.Config, p.layout.Lock} {
		if err := safePath(path, false); err != nil {
			return err
		}
	}
	if runtime.GOOS == "linux" {
		for _, name := range profiles {
			if err := safePath(filepath.Join(p.home, name), false); err != nil {
				return err
			}
		}
	}
	// XDG locations may be customized, but configuration must never be inside a removed tree.
	for _, directory := range p.directories() {
		if within(directory, p.layout.Config) || within(directory, p.launcher) || within(directory, p.home) {
			return fmt.Errorf("overlapping installation paths: %s", directory)
		}
	}
	return nil
}

func safePath(path string, directory bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("refusing non-absolute or unclean installation path: %s", path)
	}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			wantDirectory := directory || current != path
			if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || (wantDirectory && !info.IsDir()) || (!wantDirectory && !info.Mode().IsRegular()) {
				return fmt.Errorf("refusing unsafe installation path: %s", current)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

func within(directory, path string) bool {
	if runtime.GOOS == "windows" {
		directory, path = strings.ToLower(directory), strings.ToLower(path)
	}
	relative, err := filepath.Rel(directory, path)
	return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)))
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func removeEmptyDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(path)
	}
	return nil
}

var profiles = []string{".profile", ".bash_profile", ".bash_login"}
