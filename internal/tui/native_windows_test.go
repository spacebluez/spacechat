package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"xchat/internal/client"
	"xchat/internal/securestore"
	"xchat/internal/server"
)

type terminalCapture struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (capture *terminalCapture) Write(data []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.buffer.Write(data)
}
func (capture *terminalCapture) contains(text string) bool {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return strings.Contains(capture.buffer.String(), text)
}
func eventually(t *testing.T, description string, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal(description)
}
func TestWindowsExecutableInPseudoTerminal(t *testing.T) {
	executable := os.Getenv("XCHAT_EXE")
	if executable == "" {
		t.Skip("set XCHAT_EXE to test the built Windows executable")
	}
	for _, size := range []windows.Coord{{X: 100, Y: 30}, {X: 60, Y: 20}, {X: 40, Y: 18}} {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) { testWindowsExecutableInPseudoTerminal(t, executable, size) })
	}
}
func testWindowsExecutableInPseudoTerminal(t *testing.T, executable string, size windows.Coord) {
	database, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository, err := database.Room("test-access-key")
	if err != nil {
		t.Fatal(err)
	}
	service := server.NewRooms(database)
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	if err = windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(inputRead)
	if err = windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(outputWrite)
	input := os.NewFile(uintptr(inputWrite), "terminal-input")
	defer input.Close()
	output := os.NewFile(uintptr(outputRead), "terminal-output")
	defer output.Close()
	var pseudo windows.Handle
	if err = windows.CreatePseudoConsole(size, inputRead, outputWrite, 0, &pseudo); err != nil {
		t.Fatal(err)
	}
	defer windows.ClosePseudoConsole(pseudo)
	capture := &terminalCapture{}
	go io.Copy(capture, output)
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatal(err)
	}
	defer attributes.Delete()
	update := windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")
	result, _, callError := update.Call(uintptr(unsafe.Pointer(attributes.List())), 0, windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(pseudo), unsafe.Sizeof(pseudo), 0, 0)
	if result == 0 {
		t.Fatal(callError)
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	command, err := windows.UTF16PtrFromString("\"" + executable + "\" --server ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws")
	if err != nil {
		t.Fatal(err)
	}
	var process windows.ProcessInformation
	if err = windows.CreateProcess(nil, command, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &process); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(process.Process)
	defer windows.CloseHandle(process.Thread)
	defer windows.TerminateProcess(process.Process, 1)
	eventually(t, "login screen not rendered", func() bool { return capture.contains("SpaceChat") })
	time.Sleep(650 * time.Millisecond)
	if err = windows.ResizePseudoConsole(pseudo, windows.Coord{X: 24, Y: 10}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if err = windows.ResizePseudoConsole(pseudo, size); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	screen := consoleScreen(t, process.ProcessId)
	t.Log("visible login screen checked after resize and blinking")
	labels := 0
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "房间口令") && !strings.Contains(line, "输入") {
			labels++
		}
	}
	if labels != 1 {
		t.Fatalf("expected one visible room label, got %d", labels)
	}
	if _, err = io.WriteString(input, "终端验收\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "nickname was not rendered", func() bool { return capture.contains("终端验收") })
	time.Sleep(650 * time.Millisecond)
	if capture.contains("\x00") {
		t.Fatal("focus switch emitted NUL characters to Windows terminal")
	}
	if _, err = io.WriteString(input, "test-access-key\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "nickname entry did not connect", func() bool { return capture.contains("已连接") })
	if capture.contains("test-access-key") {
		t.Fatal("access key leaked to terminal output")
	}
	if _, err = io.WriteString(input, "Windows 中文终端验收\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "Chinese terminal message was not saved", func() bool { latest, err := repository.LatestID(); return err == nil && latest == 1 })
	page, err := repository.Page(0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Messages[0].Nickname != "终端验收" || page.Messages[0].Body != "Windows 中文终端验收" {
		t.Fatalf("text changed: %+v", page)
	}
	sendConsoleText(t, process.ProcessId, "first line")
	sendConsoleKey(t, process.ProcessId, 13, '\r', 16)
	sendConsoleText(t, process.ProcessId, "second line")
	time.Sleep(300 * time.Millisecond)
	if latest, _ := repository.LatestID(); latest != 1 {
		t.Fatal("Shift+Enter sent instead of newline")
	}
	if _, err = io.WriteString(input, "\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "multiline message not saved", func() bool { latest, _ := repository.LatestID(); return latest == 2 })
	page, err = repository.Page(0, 0, 2)
	if err != nil || page.Messages[1].Body != "first line\nsecond line" {
		t.Fatalf("native Shift+Enter failed: %+v %v", page, err)
	}
	if _, err = io.WriteString(input, "\x1b[200~paste one\npaste two\x1b[201~"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if latest, _ := repository.LatestID(); latest != 2 {
		t.Fatal("paste sent automatically")
	}
	if _, err = io.WriteString(input, "\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "paste not saved", func() bool { latest, _ := repository.LatestID(); return latest == 3 })
	page, err = repository.Page(0, 0, 3)
	if err != nil || page.Messages[2].Body != "paste one\npaste two" {
		t.Fatalf("paste flattened: %+v %v", page, err)
	}
	if _, err = io.WriteString(input, "\x1bOQ"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "F2 did not open switch dialog", func() bool { return capture.contains("切换聊天室") })
	capture.mu.Lock()
	beforeCancel := capture.buffer.Len()
	capture.mu.Unlock()
	if _, err = io.WriteString(input, "\x1b"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "Escape did not cancel switch dialog", func() bool {
		capture.mu.Lock()
		defer capture.mu.Unlock()
		return strings.Contains(capture.buffer.String()[beforeCancel:], "SpaceChat")
	})
	if _, err = io.WriteString(input, "\x1bOQswitch-target\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "client did not switch rooms", func() bool { return capture.contains("已切换聊天室") })
	target, err := database.Room("switch-target")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(input, "新房间验收\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "new room message not saved", func() bool { latest, err := target.LatestID(); return err == nil && latest == 4 })
	if latest, err := repository.LatestID(); err != nil || latest != 3 {
		t.Fatal("new room message leaked to old room")
	}
	if capture.contains("switch-target") {
		t.Fatal("target passphrase leaked into terminal output")
	}
	if _, err = io.WriteString(input, "keep-draft\x1bOR"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "F3 did not open kaomoji search", func() bool { return capture.contains("颜文字") })
	if _, err = io.WriteString(input, "hello\x13"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "kaomoji quick send failed", func() bool { latest, _ := target.LatestID(); return latest == 5 })
	page, err = target.Page(0, 0, 5)
	if err != nil || page.Messages[len(page.Messages)-1].Body != "(｡•ᴗ•｡)ﾉ" {
		t.Fatal("wrong kaomoji sent", page, err)
	}
	if _, err = io.WriteString(input, "\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "quick send lost draft", func() bool { latest, _ := target.LatestID(); return latest == 6 })
	page, err = target.Page(0, 0, 6)
	if err != nil || page.Messages[len(page.Messages)-1].Body != "keep-draft" {
		t.Fatal("draft changed", page, err)
	}
	if _, err = io.WriteString(input, "\x1b[15~"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "F5 did not open recall picker", func() bool { return capture.contains("撤回消息") })
	if _, err = io.WriteString(input, "\r\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "recall was not persisted", func() bool {
		page, err := target.Page(0, 0, 6)
		return err == nil && len(page.Messages) > 0 && page.Messages[len(page.Messages)-1].Recalled && page.Messages[len(page.Messages)-1].Body == ""
	})
	observer := client.New("ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws")
	observerContext, stopObserver := context.WithCancel(context.Background())
	observerDone, observerReady := make(chan struct{}), make(chan struct{})
	go func() { observer.Run(observerContext, "Bob Smith", "switch-target"); close(observerDone) }()
	go func() {
		ready := false
		for event := range observer.Events() {
			if !ready && event.State == "connected" {
				close(observerReady)
				ready = true
			}
		}
	}()
	defer func() { stopObserver(); <-observerDone }()
	select {
	case <-observerReady:
	case <-time.After(5 * time.Second):
		t.Fatal("observer not connected")
	}
	if _, err = io.WriteString(input, "\x1bOS"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "F4 did not open members", func() bool { return capture.contains("@成员") })
	if _, err = io.WriteString(input, "Bob\r\r"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "member mention not sent", func() bool { latest, _ := target.LatestID(); return latest == 7 })
	page, err = target.Page(0, 0, 7)
	if err != nil {
		t.Fatal(err)
	}
	mentioned := page.Messages[len(page.Messages)-1]
	if len(mentioned.Mentions) != 1 || mentioned.Mentions[0] != "Bob Smith" {
		t.Fatal("mention metadata missing", mentioned)
	}
	if err = windows.ResizePseudoConsole(pseudo, windows.Coord{X: 40, Y: 18}); err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(input, "\x1b[5~\x1b[6~\x03"); err != nil {
		t.Fatal(err)
	}
	status, err := windows.WaitForSingleObject(process.Process, 5000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("client did not quit: %d %v", status, err)
	}
	var exitCode uint32
	if err = windows.GetExitCodeProcess(process.Process, &exitCode); err != nil || exitCode != 0 {
		t.Fatalf("client exit: %d %v", exitCode, err)
	}
	t.Log("packaged executable rendered, joined, switched rooms, sent kaomoji, preserved draft, recalled, mentioned a member and exited in Windows ConPTY")
}
