package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
	Shared   bool
}

func (installer Installer) Install(ctx context.Context, check Check) (result InstallResult, resultError error) {
	lock, err := Acquire(installer.Layout, time.Now())
	if err != nil {
		if errors.Is(err, ErrUpdateLocked) {
			return installer.completedByAnother(check)
		}
		return InstallResult{}, err
	}
	defer lock.Release()
	active, err := ReadCurrent(installer.Layout)
	if err != nil {
		return InstallResult{}, err
	}
	targetPath, err := installer.Layout.ClientPath(check.Latest, installer.GOOS)
	if err != nil {
		return InstallResult{}, err
	}
	targetDirectory := filepath.Dir(targetPath)
	if active == check.Latest {
		if err = verifyInstalledArtifact(targetPath, check.Artifact); err != nil {
			return InstallResult{}, fmt.Errorf("verify concurrently installed client: %w", err)
		}
		return InstallResult{Path: targetPath, Previous: check.Current, Target: check.Latest, Shared: true}, nil
	}
	if active != check.Current {
		return InstallResult{}, errors.New("active client version changed during update")
	}
	if targetInfo, statError := os.Lstat(targetDirectory); statError == nil {
		if !targetInfo.IsDir() || targetInfo.Mode()&os.ModeSymlink != 0 {
			return InstallResult{}, errors.New("inactive target version is not a safe directory")
		}
		if verifyError := verifyInstalledArtifact(targetPath, check.Artifact); verifyError == nil {
			if checkError := installer.runSelfCheck(ctx, targetPath); checkError == nil {
				if err = WriteCurrent(installer.Layout, check.Latest); err != nil {
					return InstallResult{}, err
				}
				return InstallResult{Path: targetPath, Previous: active, Target: check.Latest}, nil
			}
		}
		if err = os.RemoveAll(targetDirectory); err != nil {
			return InstallResult{}, fmt.Errorf("remove incomplete target version: %w", err)
		}
	} else if !errors.Is(statError, os.ErrNotExist) {
		return InstallResult{}, statError
	}
	if err = os.MkdirAll(installer.Layout.Versions, 0700); err != nil {
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
	stagingDirectory, err := os.MkdirTemp(installer.Layout.Versions, "."+check.Latest.String()+"-")
	if err != nil {
		return InstallResult{}, err
	}
	stagingPath := filepath.Join(stagingDirectory, filepath.Base(targetPath))
	promoted := false
	defer func() {
		if stagingDirectory != "" {
			_ = os.RemoveAll(stagingDirectory)
		}
		if resultError != nil && promoted {
			os.RemoveAll(targetDirectory)
		}
	}()
	if err = os.Rename(temporaryPath, stagingPath); err != nil {
		return InstallResult{}, err
	}
	if installer.GOOS == "linux" {
		if err = os.Chmod(stagingPath, 0755); err != nil {
			return InstallResult{}, err
		}
	}
	if err = installer.runSelfCheck(ctx, stagingPath); err != nil {
		return InstallResult{}, fmt.Errorf("new client self-check: %w", err)
	}
	if err = os.Rename(stagingDirectory, targetDirectory); err != nil {
		return InstallResult{}, err
	}
	stagingDirectory = ""
	promoted = true
	if err = WriteCurrent(installer.Layout, check.Latest); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Path: targetPath, Previous: active, Target: check.Latest}, nil
}

func (installer Installer) runSelfCheck(ctx context.Context, path string) error {
	if installer.SelfCheck != nil {
		return installer.SelfCheck(ctx, path)
	}
	command := exec.CommandContext(ctx, path, "--self-check")
	return command.Run()
}

func (installer Installer) completedByAnother(check Check) (InstallResult, error) {
	active, err := ReadCurrent(installer.Layout)
	if err != nil || active != check.Latest {
		return InstallResult{}, ErrUpdateLocked
	}
	targetPath, err := installer.Layout.ClientPath(check.Latest, installer.GOOS)
	if err != nil {
		return InstallResult{}, err
	}
	if err = verifyInstalledArtifact(targetPath, check.Artifact); err != nil {
		return InstallResult{}, fmt.Errorf("verify concurrently installed client: %w", err)
	}
	confirmed, err := ReadCurrent(installer.Layout)
	if err != nil || confirmed != check.Latest {
		return InstallResult{}, ErrUpdateLocked
	}
	return InstallResult{Path: targetPath, Previous: check.Current, Target: check.Latest, Shared: true}, nil
}

func verifyInstalledArtifact(path string, artifact Artifact) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != artifact.Size {
		return errors.New("installed artifact metadata does not match manifest")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return errors.New("installed artifact digest does not match manifest")
	}
	return nil
}

func (installer Installer) Rollback(result InstallResult) error {
	if result.Shared {
		return errors.New("refusing to roll back another updater's installation")
	}
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
