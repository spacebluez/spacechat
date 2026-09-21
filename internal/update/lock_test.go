package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateLockExcludesConcurrentWriterAndReleases(t *testing.T) {
	layout := testInstallLayout(t)
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	first, err := Acquire(layout, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Acquire(layout, now.Add(time.Minute)); err == nil {
		t.Fatal("fresh lock did not exclude a second writer")
	}
	if err = first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(layout, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = second.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateLockRecoversStaleOwnedFile(t *testing.T) {
	layout := testInstallLayout(t)
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if _, err := Acquire(layout, now.Add(-11*time.Minute)); err != nil {
		t.Fatal(err)
	}
	lock, err := Acquire(layout, now)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
}

func TestUpdateLockRejectsUnknownAndSymlinkFiles(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T, layout Layout){
		"invalid": func(t *testing.T, layout Layout) {
			if err := os.WriteFile(layout.Lock, []byte("not lock metadata"), 0600); err != nil {
				t.Fatal(err)
			}
		},
		"symlink": func(t *testing.T, layout Layout) {
			target := filepath.Join(layout.Root, "target")
			if err := os.WriteFile(target, []byte("target"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, layout.Lock); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			layout := testInstallLayout(t)
			if err := os.MkdirAll(layout.Root, 0700); err != nil {
				t.Fatal(err)
			}
			setup(t, layout)
			if _, err := Acquire(layout, time.Now()); err == nil {
				t.Fatal("unsafe lock file accepted")
			}
		})
	}
}
