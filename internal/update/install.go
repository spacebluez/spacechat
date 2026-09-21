package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Installer struct {
	Layout    Layout
	Remote    Remote
	GOOS      string
	SelfCheck func(context.Context, string) error
	Progress  func(received, total int64)
}

type InstallResult struct {
	Path     string
	Previous Version
	Target   Version
}

func (installer Installer) Install(ctx context.Context, check Check) (result InstallResult, resultError error) {
	lock, err := Acquire(installer.Layout, time.Now())
	if err != nil {
		return InstallResult{}, err
	}
	defer lock.Release()
	active, err := ReadCurrent(installer.Layout)
	if err != nil {
		return InstallResult{}, err
	}
	if active != check.Current {
		return InstallResult{}, errors.New("active client version changed during update")
	}
	targetPath, err := installer.Layout.ClientPath(check.Latest, installer.GOOS)
	if err != nil {
		return InstallResult{}, err
	}
	targetDirectory := filepath.Dir(targetPath)
	if _, err = os.Lstat(targetDirectory); err == nil {
		return InstallResult{}, errors.New("target client version already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return InstallResult{}, err
	}
	if err = os.MkdirAll(installer.Layout.Temp, 0700); err != nil {
		return InstallResult{}, err
	}
	temporary, err := os.CreateTemp(installer.Layout.Temp, ".download-")
	if err != nil {
		return InstallResult{}, err
	}
	temporaryPath := temporary.Name()
	if err = temporary.Close(); err != nil {
		os.Remove(temporaryPath)
		return InstallResult{}, err
	}
	if err = os.Remove(temporaryPath); err != nil {
		return InstallResult{}, err
	}
	defer os.Remove(temporaryPath)
	if err = installer.Remote.Download(ctx, check.Server, check.Artifact, temporaryPath, installer.Progress); err != nil {
		return InstallResult{}, err
	}
	if err = os.MkdirAll(targetDirectory, 0700); err != nil {
		return InstallResult{}, err
	}
	installed := false
	defer func() {
		if resultError != nil && !installed {
			os.RemoveAll(targetDirectory)
		}
	}()
	if installer.GOOS == "linux" {
		if err = os.Chmod(temporaryPath, 0755); err != nil {
			return InstallResult{}, err
		}
	}
	if err = os.Rename(temporaryPath, targetPath); err != nil {
		return InstallResult{}, err
	}
	selfCheck := installer.SelfCheck
	if selfCheck == nil {
		selfCheck = func(ctx context.Context, path string) error {
			command := exec.CommandContext(ctx, path, "--self-check")
			return command.Run()
		}
	}
	if err = selfCheck(ctx, targetPath); err != nil {
		return InstallResult{}, fmt.Errorf("new client self-check: %w", err)
	}
	if err = WriteCurrent(installer.Layout, check.Latest); err != nil {
		return InstallResult{}, err
	}
	installed = true
	return InstallResult{Path: targetPath, Previous: active, Target: check.Latest}, nil
}

func (installer Installer) Rollback(result InstallResult) error {
	lock, err := Acquire(installer.Layout, time.Now())
	if err != nil {
		return err
	}
	defer lock.Release()
	active, err := ReadCurrent(installer.Layout)
	if err != nil {
		return err
	}
	if active != result.Target {
		return errors.New("refusing to roll back a different active version")
	}
	if err = WriteCurrent(installer.Layout, result.Previous); err != nil {
		return err
	}
	if directory := filepath.Dir(result.Path); directory != installer.Layout.Versions {
		_ = os.RemoveAll(directory)
	}
	return nil
}

func Cleanup(layout Layout, current Version) error {
	if err := cleanDirectoryContents(layout.Temp); err != nil {
		return err
	}
	entries, err := os.ReadDir(layout.Versions)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var previous *Version
	for _, entry := range entries {
		version, parseError := ParseVersion(entry.Name())
		if parseError != nil || !entry.IsDir() || version == current {
			continue
		}
		if previous == nil || version.Compare(*previous) > 0 {
			copyOf := version
			previous = &copyOf
		}
	}
	for _, entry := range entries {
		version, parseError := ParseVersion(entry.Name())
		keep := parseError == nil && entry.IsDir() && (version == current || previous != nil && version == *previous)
		if keep {
			continue
		}
		if err = os.RemoveAll(filepath.Join(layout.Versions, entry.Name())); err != nil && !errors.Is(err, os.ErrPermission) {
			return err
		}
	}
	return nil
}

func cleanDirectoryContents(directory string) error {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err = os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, os.ErrPermission) {
			return err
		}
	}
	return nil
}
