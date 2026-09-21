package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Layout struct {
	Root     string
	Bin      string
	Config   string
	Current  string
	Versions string
	Temp     string
	Lock     string
}

func LayoutFor(goos, home string, getenv func(string) string) (Layout, error) {
	var root, bin, config string
	switch goos {
	case "windows":
		local := getenv("LOCALAPPDATA")
		if local == "" {
			return Layout{}, errors.New("LOCALAPPDATA is required")
		}
		root = joinForOS(goos, local, "SpaceChat")
		bin = joinForOS(goos, root, "bin")
		config = joinForOS(goos, root, "config.json")
	case "linux":
		if home == "" {
			return Layout{}, errors.New("home directory is required")
		}
		data := getenv("XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(home, ".local", "share")
		}
		configuration := getenv("XDG_CONFIG_HOME")
		if configuration == "" {
			configuration = filepath.Join(home, ".config")
		}
		root = filepath.Join(data, "spacechat")
		bin = filepath.Join(home, ".local", "bin")
		config = filepath.Join(configuration, "spacechat", "config.json")
	default:
		return Layout{}, fmt.Errorf("unsupported operating system %s", goos)
	}
	return Layout{
		Root:     root,
		Bin:      bin,
		Config:   config,
		Current:  joinForOS(goos, root, "current"),
		Versions: joinForOS(goos, root, "versions"),
		Temp:     joinForOS(goos, root, "tmp"),
		Lock:     joinForOS(goos, root, "update.lock"),
	}, nil
}

func joinForOS(goos, first string, rest ...string) string {
	if goos != "windows" {
		parts := append([]string{first}, rest...)
		return filepath.Join(parts...)
	}
	result := strings.TrimRight(strings.ReplaceAll(first, "/", `\`), `\`)
	for _, part := range rest {
		part = strings.Trim(strings.ReplaceAll(part, "/", `\`), `\`)
		if part != "" {
			result += `\` + part
		}
	}
	return result
}

func (layout Layout) ClientPath(version Version, goos string) (string, error) {
	name := "spacechat-client"
	if goos == "windows" {
		name += ".exe"
	} else if goos != "linux" {
		return "", fmt.Errorf("unsupported operating system %s", goos)
	}
	return joinForOS(goos, layout.Versions, version.String(), name), nil
}

func ReadCurrent(layout Layout) (Version, error) {
	info, err := os.Lstat(layout.Current)
	if err != nil {
		return Version{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 64 {
		return Version{}, errors.New("current version pointer is invalid")
	}
	contents, err := os.ReadFile(layout.Current)
	if err != nil {
		return Version{}, err
	}
	text := string(contents)
	text = strings.TrimSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\r")
	if strings.ContainsAny(text, "\r\n") {
		return Version{}, errors.New("current version pointer contains multiple lines")
	}
	return ParseVersion(text)
}

func WriteCurrent(layout Layout, version Version) (result error) {
	if err := os.MkdirAll(filepath.Dir(layout.Current), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(layout.Current), ".current-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() {
		temporary.Close()
		if result != nil {
			os.Remove(name)
		}
	}()
	if err = temporary.Chmod(0600); err != nil {
		return err
	}
	if _, err = temporary.WriteString(version.String() + "\n"); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = replaceFile(name, layout.Current); err != nil {
		return err
	}
	return nil
}
