package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testInstallLayout(t *testing.T) Layout {
	t.Helper()
	root := t.TempDir()
	return Layout{
		Root:     root,
		Current:  filepath.Join(root, "current"),
		Versions: filepath.Join(root, "versions"),
		Temp:     filepath.Join(root, "tmp"),
		Lock:     filepath.Join(root, "update.lock"),
	}
}

func TestInstallerSwitchesOnlyAfterSelfCheckAndCanRollback(t *testing.T) {
	server, publicKey := signedRemoteServer(t, []byte("windows"), []byte("linux"))
	current, _ := ParseVersion("0.3.2")
	layout := testInstallLayout(t)
	if err := WriteCurrent(layout, current); err != nil {
		t.Fatal(err)
	}
	remote := Remote{PublicKey: publicKey}
	check, err := remote.Check(context.Background(), websocketAddress(server), current, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	selfChecked := false
	installer := Installer{
		Layout: layout,
		Remote: remote,
		GOOS:   "linux",
		SelfCheck: func(ctx context.Context, path string) error {
			selfChecked = true
			contents, err := os.ReadFile(path)
			if err != nil || string(contents) != "linux" {
				t.Fatalf("self-check contents = %q, %v", contents, err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm()&0100 == 0 {
				t.Fatalf("client is not executable: %v, %v", info, err)
			}
			return nil
		},
	}
	result, err := installer.Install(context.Background(), check)
	if err != nil {
		t.Fatal(err)
	}
	if !selfChecked || result.Previous != current || result.Target != check.Latest {
		t.Fatalf("wrong install result: %+v", result)
	}
	active, err := ReadCurrent(layout)
	if err != nil || active != check.Latest {
		t.Fatalf("active = %v, %v", active, err)
	}
	if err = installer.Rollback(result); err != nil {
		t.Fatal(err)
	}
	active, err = ReadCurrent(layout)
	if err != nil || active != current {
		t.Fatalf("rollback active = %v, %v", active, err)
	}
}

func TestInstallerFailurePreservesCurrentVersion(t *testing.T) {
	server, publicKey := signedRemoteServer(t, []byte("windows"), []byte("linux"))
	current, _ := ParseVersion("0.3.2")
	layout := testInstallLayout(t)
	if err := WriteCurrent(layout, current); err != nil {
		t.Fatal(err)
	}
	remote := Remote{PublicKey: publicKey}
	check, err := remote.Check(context.Background(), websocketAddress(server), current, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	installer := Installer{
		Layout: layout,
		Remote: remote,
		GOOS:   "linux",
		SelfCheck: func(context.Context, string) error {
			return errors.New("self-check failed")
		},
	}
	if _, err = installer.Install(context.Background(), check); err == nil {
		t.Fatal("self-check failure accepted")
	}
	active, err := ReadCurrent(layout)
	if err != nil || active != current {
		t.Fatalf("active changed to %v, %v", active, err)
	}
	target, _ := layout.ClientPath(check.Latest, "linux")
	if _, err = os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("failed target remains: %v", err)
	}
	if _, err = os.Stat(layout.Lock); !os.IsNotExist(err) {
		t.Fatalf("update lock remains: %v", err)
	}
}

func TestCleanupKeepsCurrentAndNewestPriorVersion(t *testing.T) {
	layout := testInstallLayout(t)
	for _, version := range []string{"0.2.0", "0.3.0", "0.4.0", "invalid"} {
		if err := os.MkdirAll(filepath.Join(layout.Versions, version), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(layout.Temp, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.Temp, "partial"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	current, _ := ParseVersion("0.4.0")
	if err := Cleanup(layout, current); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"0.3.0", "0.4.0"} {
		if _, err := os.Stat(filepath.Join(layout.Versions, version)); err != nil {
			t.Fatalf("retained version %s missing: %v", version, err)
		}
	}
	for _, version := range []string{"0.2.0", "invalid"} {
		if _, err := os.Stat(filepath.Join(layout.Versions, version)); !os.IsNotExist(err) {
			t.Fatalf("obsolete version %s remains: %v", version, err)
		}
	}
	entries, err := os.ReadDir(layout.Temp)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary files remain: %v, %v", entries, err)
	}
}
