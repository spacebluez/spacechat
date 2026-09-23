package uninstall

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func checkRunning(p plan) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		executable, err := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if err == nil && (within(p.layout.Root, executable) || executable == p.launcher) {
			return fmt.Errorf("close all SpaceChat clients before uninstalling (process %d)", pid)
		}
	}
	return nil
}

func cleanPath(p plan) error {
	for _, name := range profiles {
		path := filepath.Join(p.home, name)
		contents, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		cleaned := stripProfileBlock(contents)
		if bytes.Equal(contents, cleaned) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if err := replaceProfile(path, cleaned, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func stripProfileBlock(contents []byte) []byte {
	lines := bytes.SplitAfter(contents, []byte("\n"))
	var result []byte
	for i := 0; i < len(lines); i++ {
		if i+1 < len(lines) && string(bytes.TrimRight(lines[i], "\r\n")) == "# Added by SpaceChat installer" &&
			string(bytes.TrimRight(lines[i+1], "\r\n")) == `export PATH="$HOME/.local/bin:$PATH"` {
			i++
			continue
		}
		result = append(result, lines[i]...)
	}
	return result
}

func replaceProfile(path string, contents []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".spacechat-profile-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func removeLauncher(p plan) (bool, error) {
	return false, removeFile(p.launcher)
}
