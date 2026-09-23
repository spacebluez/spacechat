package uninstall

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallRemovesOnlyInstallerProfileBlocks(t *testing.T) {
	layout, _ := installedLayout(t)
	home, _ := os.UserHomeDir()
	before := "export EXISTING=value\n"
	block := "# Added by SpaceChat installer\nexport PATH=\"$HOME/.local/bin:$PATH\"\n"
	after := "# My own PATH\nexport PATH=\"$HOME/.local/bin:$PATH\"\n"
	for _, name := range profiles {
		writeFile(t, filepath.Join(home, name), before+block+after)
	}
	if err := Run(layout, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, name := range profiles {
		assertContents(t, filepath.Join(home, name), before+after)
	}
}

func TestUninstallRejectsSymlinkedProfileBeforeDeleting(t *testing.T) {
	layout, launcher := installedLayout(t)
	home, _ := os.UserHomeDir()
	external := filepath.Join(t.TempDir(), "profile")
	writeFile(t, external, "keep")
	if err := os.Symlink(external, filepath.Join(home, ".profile")); err != nil {
		t.Fatal(err)
	}
	if err := Run(layout, []string{"--purge"}, io.Discard); err == nil {
		t.Fatal("symlinked profile accepted")
	}
	assertContents(t, launcher, "fixture")
	assertContents(t, external, "keep")
}
