package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLayoutForWindowsAndLinux(t *testing.T) {
	windows, err := LayoutFor("windows", `C:\Users\alice`, func(name string) string {
		if name == "LOCALAPPDATA" {
			return `C:\Users\alice\AppData\Local`
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if windows.Root != `C:\Users\alice\AppData\Local\SpaceChat` || windows.Bin != `C:\Users\alice\AppData\Local\SpaceChat\bin` || windows.Config != `C:\Users\alice\AppData\Local\SpaceChat\config.json` {
		t.Fatalf("wrong Windows layout: %+v", windows)
	}

	linux, err := LayoutFor("linux", "/home/alice", func(name string) string {
		switch name {
		case "XDG_CONFIG_HOME":
			return "/cfg"
		case "XDG_DATA_HOME":
			return "/data"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if linux.Root != "/data/spacechat" || linux.Bin != "/home/alice/.local/bin" || linux.Config != "/cfg/spacechat/config.json" {
		t.Fatalf("wrong Linux layout: %+v", linux)
	}
}

func TestLayoutUsesLinuxXDGDefaultsAndRejectsMissingWindowsRoot(t *testing.T) {
	layout, err := LayoutFor("linux", "/home/alice", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if layout.Root != "/home/alice/.local/share/spacechat" || layout.Config != "/home/alice/.config/spacechat/config.json" {
		t.Fatalf("wrong defaults: %+v", layout)
	}
	if _, err = LayoutFor("windows", `C:\Users\alice`, func(string) string { return "" }); err == nil {
		t.Fatal("accepted missing LOCALAPPDATA")
	}
	if _, err = LayoutFor("darwin", "/Users/alice", func(string) string { return "" }); err == nil {
		t.Fatal("accepted unsupported OS")
	}
}

func TestCurrentVersionRoundTripAndValidation(t *testing.T) {
	root := t.TempDir()
	layout := Layout{
		Root:     root,
		Current:  filepath.Join(root, "current"),
		Versions: filepath.Join(root, "versions"),
		Temp:     filepath.Join(root, "tmp"),
		Lock:     filepath.Join(root, "update.lock"),
	}
	first, _ := ParseVersion("0.3.2")
	second, _ := ParseVersion("0.4.0")
	if err := WriteCurrent(layout, first); err != nil {
		t.Fatal(err)
	}
	if err := WriteCurrent(layout, second); err != nil {
		t.Fatal(err)
	}
	actual, err := ReadCurrent(layout)
	if err != nil || actual != second {
		t.Fatalf("ReadCurrent = %v, %v", actual, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "current" {
			t.Fatalf("temporary pointer remained: %s", entry.Name())
		}
	}
	for _, contents := range []string{"../escape\n", "0.4.0/other\n", "v0.4.0\n", "0.4.0\nextra\n"} {
		if err = os.WriteFile(layout.Current, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = ReadCurrent(layout); err == nil {
			t.Fatalf("accepted current contents %q", contents)
		}
	}
}

func TestManagedClientPath(t *testing.T) {
	version, _ := ParseVersion("0.4.0")
	windows := Layout{Versions: `C:\Users\alice\AppData\Local\SpaceChat\versions`}
	path, err := windows.ClientPath(version, "windows")
	if err != nil || path != `C:\Users\alice\AppData\Local\SpaceChat\versions\0.4.0\spacechat-client.exe` {
		t.Fatalf("Windows ClientPath = %q, %v", path, err)
	}
	linux := Layout{Versions: "/home/alice/.local/share/spacechat/versions"}
	path, err = linux.ClientPath(version, "linux")
	if err != nil || path != "/home/alice/.local/share/spacechat/versions/0.4.0/spacechat-client" {
		t.Fatalf("Linux ClientPath = %q, %v", path, err)
	}
	if _, err = linux.ClientPath(version, "darwin"); err == nil {
		t.Fatal("accepted unsupported OS")
	}
}
