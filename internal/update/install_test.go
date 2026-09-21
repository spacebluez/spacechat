package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestInstallerRecoversVerifiedInactiveTargetAfterInterruptedSwitch(t *testing.T) {
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
	target, _ := layout.ClientPath(check.Latest, "linux")
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(target, []byte("linux"), 0755); err != nil {
		t.Fatal(err)
	}
	selfChecks := 0
	installer := Installer{Layout: layout, Remote: remote, GOOS: "linux", SelfCheck: func(context.Context, string) error {
		selfChecks++
		return nil
	}}
	result, err := installer.Install(context.Background(), check)
	if err != nil {
		t.Fatal(err)
	}
	active, err := ReadCurrent(layout)
	if err != nil || active != check.Latest || result.Shared || selfChecks != 1 {
		t.Fatalf("active=%v result=%+v checks=%d err=%v", active, result, selfChecks, err)
	}
}

func TestInstallerReplacesCorruptInactiveTarget(t *testing.T) {
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
	target, _ := layout.ClientPath(check.Latest, "linux")
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(target, []byte("xxxxx"), 0755); err != nil {
		t.Fatal(err)
	}
	installer := Installer{Layout: layout, Remote: remote, GOOS: "linux", SelfCheck: func(context.Context, string) error { return nil }}
	if _, err = installer.Install(context.Background(), check); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "linux" {
		t.Fatalf("recovered target=%q err=%v", contents, err)
	}
}

func TestInstallerReusesTargetCompletedByConcurrentUpdater(t *testing.T) {
	layout := testInstallLayout(t)
	oldVersion, _ := ParseVersion("0.3.2")
	latest, _ := ParseVersion("0.4.0")
	if err := WriteCurrent(layout, latest); err != nil {
		t.Fatal(err)
	}
	target, _ := layout.ClientPath(latest, "linux")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	contents := []byte("linux")
	if err := os.WriteFile(target, contents, 0755); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(contents)
	check := Check{
		Current: oldVersion, Latest: latest,
		Artifact: Artifact{File: filepath.Base(target), Size: int64(len(contents)), SHA256: hex.EncodeToString(hash[:])},
	}
	lock, err := Acquire(layout, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	result, err := (Installer{Layout: layout, GOOS: "linux"}).Install(context.Background(), check)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Shared || result.Path != target || result.Target != latest {
		t.Fatalf("concurrent result = %+v", result)
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
