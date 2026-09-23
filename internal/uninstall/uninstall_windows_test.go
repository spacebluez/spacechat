package uninstall

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestUninstallPreservesOtherPathEntriesAndRegistryType(t *testing.T) {
	path := fmt.Sprintf(`Software\SpaceChat-Uninstall-Test-%d`, time.Now().UnixNano())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	defer key.Close()
	bin := `C:\Users\Tester\AppData\Local\SpaceChat\bin`
	other := `%USERPROFILE%\tools;;C:\unrelated;C:\SpaceChat\bin-extra`
	for _, expandable := range []bool{false, true} {
		value := bin + ";" + other + ";\"" + strings.ToUpper(bin) + "\\\""
		kind := uint32(registry.SZ)
		if expandable {
			err = key.SetExpandStringValue("Path", value)
			kind = registry.EXPAND_SZ
		} else {
			err = key.SetStringValue("Path", value)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := cleanUserPath(key, bin); err != nil {
			t.Fatal(err)
		}
		actual, actualKind, err := key.GetStringValue("Path")
		if err != nil || actual != other || actualKind != kind {
			t.Fatalf("PATH = %q, type %d, err %v", actual, actualKind, err)
		}
	}
}
