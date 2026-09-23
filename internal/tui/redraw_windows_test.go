package tui

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"xchat/internal/securestore"
	"xchat/internal/server"
)

func TestLegacyConsoleChatRedraw(t *testing.T) {
	executable := os.Getenv("XCHAT_EXE")
	if executable == "" {
		t.Skip("set XCHAT_EXE to test native console repainting")
	}
	database, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := server.NewRooms(database)
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	for _, size := range [][2]uint32{{120, 30}, {238, 63}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			room := fmt.Sprintf("redraw-%dx%d", size[0], size[1])
			repository, err := database.Room(room)
			if err != nil {
				t.Fatal(err)
			}
			command, err := windows.UTF16PtrFromString(windows.EscapeArg(executable) + " --server ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws")
			if err != nil {
				t.Fatal(err)
			}
			const startUseCountChars = 0x00000008
			startup := windows.StartupInfo{Flags: windows.STARTF_USESHOWWINDOW | startUseCountChars, XCountChars: size[0], YCountChars: size[1]}
			startup.Cb = uint32(unsafe.Sizeof(startup))
			var process windows.ProcessInformation
			if err := windows.CreateProcess(nil, command, nil, nil, false, windows.CREATE_NEW_CONSOLE, nil, nil, &startup, &process); err != nil {
				t.Fatal(err)
			}
			defer windows.CloseHandle(process.Process)
			defer windows.CloseHandle(process.Thread)
			defer windows.TerminateProcess(process.Process, 1)
			defer func() {
				if t.Failed() {
					t.Logf("console on failure:\n%s", consoleScreen(t, process.ProcessId))
				}
			}()
			time.Sleep(500 * time.Millisecond)
			eventually(t, "login screen not rendered", func() bool {
				return strings.Contains(consoleScreen(t, process.ProcessId), "SpaceChat")
			})
			logRedrawConsoleSize(t, process.ProcessId)
			sendConsoleText(t, process.ProcessId, "zmz\r")
			time.Sleep(100 * time.Millisecond)
			sendConsoleText(t, process.ProcessId, room+"\r")
			eventually(t, "client did not connect", func() bool {
				return strings.Contains(consoleScreen(t, process.ProcessId), "已连接")
			})
			// Middle dots occupy two cells with legacy Chinese console fonts.
			for index, body := range []string{"hello", "every body", strings.Repeat("·", 12) + " 中文消息"} {
				sendConsoleText(t, process.ProcessId, body+"\r")
				eventually(t, "message was not saved", func() bool {
					page, err := repository.Page(0, 0, 100)
					return err == nil && len(page.Messages) == index+1
				})
			}
			sendConsoleText(t, process.ProcessId, "\r")
			assertChatRedraw(t, process.ProcessId)
		})
	}
}

func assertChatRedraw(t *testing.T, processID uint32) {
	t.Helper()
	screen := assertVisibleChatFrame(t, processID)
	for _, text := range []string{"在线成员", "Shift+Enter 换行", "hello", "every body", "中文消息"} {
		if count := strings.Count(screen, text); count != 1 {
			t.Errorf("expected one %q, got %d", text, count)
		}
	}
	if strings.Contains(screen, "还没有消息") {
		t.Error("empty-room message survived the first message")
	}
}

func assertVisibleChatFrame(t *testing.T, processID uint32) string {
	t.Helper()
	// Include a cursor blink so differential redraws are exercised too.
	time.Sleep(800 * time.Millisecond)
	screen := consoleScreen(t, processID)
	for _, text := range []string{"SpaceChat", "输入消息", "F2 换房"} {
		if count := strings.Count(screen, text); count != 1 {
			t.Errorf("expected one %q, got %d", text, count)
		}
	}
	lines := strings.Split(screen, "\n")
	if !strings.Contains(lines[0], "SpaceChat") || !strings.Contains(lines[len(lines)-1], "F2 换房") {
		t.Error("header or footer moved out of its row")
	}
	return screen
}

func logRedrawConsoleSize(t *testing.T, processID uint32) {
	t.Helper()
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	free, attach := kernel.NewProc("FreeConsole"), kernel.NewProc("AttachConsole")
	free.Call()
	result, _, err := attach.Call(uintptr(processID))
	if result == 0 {
		t.Fatal(err)
	}
	defer func() { free.Call(); attach.Call(uintptr(^uint32(0))) }()
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(output.Fd()), &info); err != nil {
		t.Fatal(err)
	}
	codePage, err := windows.GetConsoleOutputCP()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("console buffer=%dx%d window=%+v outputCP=%d", info.Size.X, info.Size.Y, info.Window, codePage)
}
