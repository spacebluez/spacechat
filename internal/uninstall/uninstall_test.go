package uninstall

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"xchat/internal/update"
)

func installedLayout(t *testing.T) (update.Layout, string) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "user space [test]'s")
	home := filepath.Join(directory, "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(directory, "Local AppData"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(directory, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(directory, "data"))
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	layout, err := update.LayoutFor(runtime.GOOS, home, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	name := "spacechat"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	launcher := filepath.Join(layout.Bin, name)
	for _, path := range []string{launcher, layout.Current, layout.Config, filepath.Join(layout.Versions, "1.2.3", "client"), filepath.Join(layout.Temp, "download")} {
		writeFile(t, path, "fixture")
	}
	if runtime.GOOS == "windows" {
		writeFile(t, filepath.Join(layout.Root, "terminal", "settings", "settings.json"), "{}")
	}
	return layout, launcher
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0700); err != nil {
		t.Fatal(err)
	}
}

func assertMissing(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s to be removed, got %v", path, err)
		}
	}
}

func assertContents(t *testing.T, path, expected string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != expected {
		t.Fatalf("%s: got %q, %v; want %q", path, contents, err, expected)
	}
}

func TestUninstallPreservesConfigAndUnrelatedFiles(t *testing.T) {
	layout, launcher := installedLayout(t)
	unrelated := filepath.Join(layout.Bin, "another-app")
	writeFile(t, unrelated, "keep")
	var output bytes.Buffer
	if err := Run(layout, nil, &output); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, launcher, layout.Current, layout.Versions, layout.Temp, layout.Lock, filepath.Join(layout.Root, "terminal"))
	assertContents(t, layout.Config, "fixture")
	assertContents(t, unrelated, "keep")
	if !strings.Contains(output.String(), "Configuration kept at:") {
		t.Fatalf("missing config location: %s", &output)
	}
}

func TestUninstallPurgeWorksWithoutCurrentVersionAndCanRepeat(t *testing.T) {
	layout, launcher := installedLayout(t)
	if err := os.Remove(layout.Current); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := Run(layout, []string{"--purge"}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	assertMissing(t, launcher, layout.Root, layout.Config)
}

func TestUninstallRejectsArgumentsBeforeDeleting(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"unrelated"}, {"--purge", "unrelated"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			layout, launcher := installedLayout(t)
			if err := Run(layout, args, io.Discard); err == nil {
				t.Fatal("unexpected arguments accepted")
			}
			assertContents(t, launcher, "fixture")
			assertContents(t, layout.Config, "fixture")
		})
	}
}

func TestUninstallHelpNeedsNoInstallation(t *testing.T) {
	var output bytes.Buffer
	if err := Run(update.Layout{}, []string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "spacechat uninstall [--purge]") {
		t.Fatal(output.String())
	}
}

func TestUninstallRejectsActiveUpdate(t *testing.T) {
	layout, launcher := installedLayout(t)
	lock, err := update.Acquire(layout, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if err := Run(layout, []string{"--purge"}, io.Discard); !errors.Is(err, update.ErrUpdateLocked) {
		t.Fatalf("expected update lock error, got %v", err)
	}
	assertContents(t, launcher, "fixture")
	assertContents(t, layout.Config, "fixture")
}

func TestUninstallRejectsLinkedInstallationAndKeepsExternalFiles(t *testing.T) {
	layout, launcher := installedLayout(t)
	external := t.TempDir()
	writeFile(t, filepath.Join(external, "keep"), "external")
	if err := os.RemoveAll(layout.Versions); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, layout.Versions); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := Run(layout, []string{"--purge"}, io.Discard); err == nil {
		t.Fatal("linked version directory accepted")
	}
	assertContents(t, filepath.Join(external, "keep"), "external")
	assertContents(t, launcher, "fixture")
	assertContents(t, layout.Config, "fixture")
}

func TestUninstallDoesNotFollowLinksInsideVersionDirectory(t *testing.T) {
	layout, _ := installedLayout(t)
	external := t.TempDir()
	writeFile(t, filepath.Join(external, "keep"), "external")
	if err := os.Symlink(external, filepath.Join(layout.Versions, "external")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := Run(layout, []string{"--purge"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	assertContents(t, filepath.Join(external, "keep"), "external")
}

func TestUninstallRejectsForeignLayout(t *testing.T) {
	layout, launcher := installedLayout(t)
	layout.Root = t.TempDir()
	if err := Run(layout, []string{"--purge"}, io.Discard); err == nil {
		t.Fatal("foreign layout accepted")
	}
	assertContents(t, launcher, "fixture")
}

func TestUninstallRunningProcessHelper(t *testing.T) {
	if os.Getenv("SPACECHAT_UNINSTALL_TEST_PROCESS") != "1" {
		return
	}
	os.Stdout.WriteString("ready\n")
	time.Sleep(time.Minute)
	os.Exit(0)
}

func copyExecutable(t *testing.T, source, destination string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, contents, 0700); err != nil {
		t.Fatal(err)
	}
}

func TestUninstallRefusesRunningClient(t *testing.T) {
	layout, launcher := installedLayout(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client := filepath.Join(layout.Versions, "1.2.3", "running-client")
	if runtime.GOOS == "windows" {
		client += ".exe"
	}
	copyExecutable(t, executable, client)
	command := exec.Command(client, "-test.run=^TestUninstallRunningProcessHelper$")
	command.Env = append(os.Environ(), "SPACECHAT_UNINSTALL_TEST_PROCESS=1")
	pipe, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { command.Process.Kill(); command.Wait() }()
	ready, err := bufio.NewReader(pipe).ReadString('\n')
	if err != nil || ready != "ready\n" {
		t.Fatalf("client did not start: %q, %v", ready, err)
	}
	if err := Run(layout, []string{"--purge"}, io.Discard); err == nil || !strings.Contains(err.Error(), "close all SpaceChat") {
		t.Fatalf("expected running client refusal, got %v", err)
	}
	assertContents(t, launcher, "fixture")
	assertContents(t, layout.Config, "fixture")
}

func TestInstalledLauncherUninstallsItself(t *testing.T) {
	name := "spacechat"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	built := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", built, "./cmd/spacechat")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build launcher: %v\n%s", err, output)
	}
	for _, purge := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep-config", true: "purge"}[purge], func(t *testing.T) {
			layout, launcher := installedLayout(t)
			copyExecutable(t, built, launcher)
			if runtime.GOOS == "linux" {
				packageDirectory := t.TempDir()
				copyExecutable(t, built, filepath.Join(packageDirectory, "spacechat"))
				writeFile(t, filepath.Join(packageDirectory, "spacechat-client"), "#!/bin/sh\ncase \"$1\" in --version) echo 'spacechat 2.3.4' ;; --self-check) exit 0 ;; *) exit 1 ;; esac\n")
				install := exec.Command("sh", "../../deploy/client/install.sh", "ws://chat.invalid/ws", "2.3.4", packageDirectory)
				if output, err := install.CombinedOutput(); err != nil {
					t.Fatalf("install command: %v\n%s", err, output)
				}
			}
			configuration, err := os.ReadFile(layout.Config)
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"uninstall"}
			if purge {
				args = append(args, "--purge")
			}
			command := exec.Command(launcher, args...)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("uninstall command: %v\n%s", err, output)
			}
			cleanupTarget := launcher
			if runtime.GOOS == "windows" {
				cleanupTarget = layout.Bin
			}
			deadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(cleanupTarget); errors.Is(err, os.ErrNotExist) {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			assertMissing(t, cleanupTarget, launcher, layout.Versions, layout.Current, layout.Temp, layout.Lock)
			if purge {
				assertMissing(t, layout.Config)
			} else {
				assertContents(t, layout.Config, string(configuration))
			}
		})
	}
}
