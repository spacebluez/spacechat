package launcher

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"xchat/internal/update"
)

func managedLauncherLayout(t *testing.T) (update.Layout, string) {
	t.Helper()
	root := t.TempDir()
	layout := update.Layout{
		Root:     root,
		Current:  filepath.Join(root, "current"),
		Versions: filepath.Join(root, "versions"),
		Temp:     filepath.Join(root, "tmp"),
		Lock:     filepath.Join(root, "update.lock"),
	}
	version, _ := update.ParseVersion("0.4.0")
	if err := update.WriteCurrent(layout, version); err != nil {
		t.Fatal(err)
	}
	client, err := layout.ClientPath(version, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(client), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(client, []byte("fake"), 0700); err != nil {
		t.Fatal(err)
	}
	return layout, client
}

func TestRunExecutesOnlyManagedCurrentClient(t *testing.T) {
	layout, client := managedLauncherLayout(t)
	stdin := bytes.NewBufferString("input")
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	streams := Streams{Stdin: stdin, Stdout: stdout, Stderr: stderr}
	called := false
	runner := func(path string, args []string, received Streams) error {
		called = true
		if path != client || !reflect.DeepEqual(args, []string{"--server", "ws://chat/ws"}) {
			t.Fatalf("run %q with %#v", path, args)
		}
		if received.Stdin != io.Reader(stdin) || received.Stdout != io.Writer(stdout) || received.Stderr != io.Writer(stderr) {
			t.Fatal("streams were not forwarded")
		}
		return errors.New("child exit")
	}
	if err := Run(layout, "linux", []string{"--server", "ws://chat/ws"}, streams, runner); err == nil || err.Error() != "child exit" {
		t.Fatalf("Run returned %v", err)
	}
	if !called {
		t.Fatal("managed client was not run")
	}
}

func TestRunRejectsMissingAndSymlinkedClient(t *testing.T) {
	layout, client := managedLauncherLayout(t)
	called := false
	runner := func(string, []string, Streams) error { called = true; return nil }
	if err := os.Remove(client); err != nil {
		t.Fatal(err)
	}
	if err := Run(layout, "linux", nil, Streams{}, runner); err == nil {
		t.Fatal("missing client accepted")
	}
	if err := os.Symlink("/bin/true", client); err != nil {
		t.Fatal(err)
	}
	if err := Run(layout, "linux", nil, Streams{}, runner); err == nil {
		t.Fatal("symlinked client accepted")
	}
	if called {
		t.Fatal("runner called for invalid client")
	}
}

func TestUninstallHelpDoesNotLaunchOrReadCurrentClient(t *testing.T) {
	var output bytes.Buffer
	runner := func(string, []string, Streams) error {
		t.Fatal("uninstall launched the client")
		return nil
	}
	if err := Run(update.Layout{}, "linux", []string{"uninstall", "--help"}, Streams{Stdout: &output}, runner); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("spacechat uninstall [--purge]")) {
		t.Fatal(output.String())
	}
}
