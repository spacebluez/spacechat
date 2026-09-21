package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func executableProcessIDs(t *testing.T, path string) []uint32 {
	t.Helper()
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	var ids []uint32
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), filepath.Base(path)) {
			continue
		}
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if err != nil {
			continue
		}
		var buffer [32768]uint16
		size := uint32(len(buffer))
		err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size)
		windows.CloseHandle(handle)
		if err == nil && strings.EqualFold(filepath.Clean(windows.UTF16ToString(buffer[:size])), filepath.Clean(path)) {
			ids = append(ids, entry.ProcessID)
		}
	}
	return ids
}

func TestPackagedUnicodeHost(t *testing.T) {
	executable := os.Getenv("XCHAT_UNICODE_EXE")
	if executable == "" {
		t.Skip("set XCHAT_UNICODE_EXE to an isolated packaged client to test double-click hosting")
	}
	if len(executableProcessIDs(t, executable)) != 0 {
		t.Fatal("test client is already running; use an isolated package")
	}
	hostPath := filepath.Join(filepath.Dir(executable), "terminal", "WindowsTerminal.exe")
	if len(executableProcessIDs(t, hostPath)) != 0 {
		t.Fatal("test terminal is already running; use an isolated package")
	}
	command, err := windows.UTF16PtrFromString(windows.EscapeArg(executable))
	if err != nil {
		t.Fatal(err)
	}
	startup := windows.StartupInfo{}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	var initial windows.ProcessInformation
	if err := windows.CreateProcess(nil, command, nil, nil, false, windows.CREATE_NEW_CONSOLE, nil, nil, &startup, &initial); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(initial.Process)
	defer windows.CloseHandle(initial.Thread)
	defer func() {
		for _, path := range []string{executable, hostPath} {
			for _, id := range executableProcessIDs(t, path) {
				if process, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, id); err == nil {
					windows.TerminateProcess(process, 1)
					windows.CloseHandle(process)
				}
			}
		}
	}()
	if status, err := windows.WaitForSingleObject(initial.Process, 10000); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("double-click client did not hand off to portable terminal", err)
	}
	var clientID uint32
	eventually(t, "portable terminal did not start the client", func() bool {
		for _, id := range executableProcessIDs(t, executable) {
			if id != initial.ProcessId {
				clientID = id
				return true
			}
		}
		return false
	})
	time.Sleep(time.Second)
	eventually(t, "hosted client login did not render", func() bool { return strings.Contains(consoleScreen(t, clientID), "房间口令") })
	sendConsoleText(t, clientID, "font-"+time.Now().Format("150405")+"\r")
	time.Sleep(150 * time.Millisecond)
	sendConsoleText(t, clientID, fmt.Sprintf("font-check-%d\r", time.Now().UnixNano()))
	eventually(t, "hosted client did not connect to public TLS", func() bool { return strings.Contains(consoleScreen(t, clientID), "已连接") })
	sendConsoleKey(t, clientID, 0x72, 0, 0)
	eventually(t, "remote Unicode catalog did not render in host", func() bool {
		screen := consoleScreen(t, clientID)
		return strings.Contains(screen, "(˶ᵔ ᵕ ᵔ˶)") && strings.Contains(screen, "72")
	})
	if script, image := os.Getenv("XCHAT_CAPTURE_SCRIPT"), os.Getenv("XCHAT_CAPTURE_IMAGE"); script != "" && image != "" {
		capture := func(path string) {
			command := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-TerminalPath", hostPath, "-ImagePath", path)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("capture test window: %v %s", err, output)
			}
		}
		capture(image)
		sendConsoleText(t, clientID, "搞怪")
		eventually(t, "uncommon Unicode search did not settle", func() bool {
			screen := consoleScreen(t, clientID)
			return strings.Contains(screen, "1 / 12") && strings.Contains(screen, "(ಡωಡ)")
		})
		capture(strings.TrimSuffix(image, filepath.Ext(image)) + "-funny" + filepath.Ext(image))
		sendConsoleText(t, clientID, "\x15")
	}
	sendConsoleText(t, clientID, "soft")
	eventually(t, "Unicode search did not settle", func() bool { return strings.Contains(consoleScreen(t, clientID), "1 / 1") })
	sendConsoleText(t, clientID, "\x13")
	eventually(t, "hosted client did not send expression", func() bool {
		screen := consoleScreen(t, clientID)
		return strings.Contains(screen, "(˶ᵔ ᵕ ᵔ˶)") && !strings.Contains(screen, "颜文字 ·") && !strings.Contains(screen, "待确认")
	})
	sendConsoleText(t, clientID, "\x03")
	eventually(t, "hosted client did not exit", func() bool { return len(executableProcessIDs(t, executable)) == 0 })
	t.Log("double-click launched portable Windows Terminal, fetched Unicode catalog over TLS, searched, sent and exited")
}
