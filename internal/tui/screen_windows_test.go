package tui

import (
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"
)

func consoleScreen(t *testing.T, processID uint32) string {
	t.Helper()
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	free := kernel.NewProc("FreeConsole")
	attach := kernel.NewProc("AttachConsole")
	free.Call()
	result, _, err := attach.Call(uintptr(processID))
	if result == 0 {
		t.Fatalf("attach test console: %v", err)
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
	type cell struct {
		character  uint16
		attributes uint16
	}
	cells := make([]cell, int(info.Size.X)*int(info.Size.Y))
	rectangle := windows.SmallRect{Left: 0, Top: 0, Right: info.Size.X - 1, Bottom: info.Size.Y - 1}
	size := uint32(uint16(info.Size.X)) | uint32(uint16(info.Size.Y))<<16
	result, _, err = kernel.NewProc("ReadConsoleOutputW").Call(output.Fd(), uintptr(unsafe.Pointer(&cells[0])), uintptr(size), 0, uintptr(unsafe.Pointer(&rectangle)))
	if result == 0 {
		t.Fatalf("read test console: %v", err)
	}
	var lines []string
	for row := 0; row < int(info.Size.Y); row++ {
		var text []uint16
		for column := 0; column < int(info.Size.X); column++ {
			cell := cells[row*int(info.Size.X)+column]
			if cell.attributes&0x200 == 0 {
				text = append(text, cell.character)
			}
		}
		lines = append(lines, strings.TrimRight(string(utf16.Decode(text)), " \x00"))
	}
	return strings.Join(lines, "\n")
}

func TestLegacyConsoleLogin(t *testing.T) {
	executable := os.Getenv("XCHAT_EXE")
	if executable == "" {
		t.Skip("set XCHAT_EXE")
	}
	command, err := windows.UTF16PtrFromString("\"" + executable + "\" --server ws://127.0.0.1:18081/ws")
	if err != nil {
		t.Fatal(err)
	}
	startup := windows.StartupInfo{}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESHOWWINDOW
	startup.ShowWindow = 0
	var process windows.ProcessInformation
	if err = windows.CreateProcess(nil, command, nil, nil, false, windows.CREATE_NEW_CONSOLE, nil, nil, &startup, &process); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(process.Process)
	defer windows.CloseHandle(process.Thread)
	defer windows.TerminateProcess(process.Process, 1)
	time.Sleep(1800 * time.Millisecond)
	screen := consoleScreen(t, process.ProcessId)
	t.Log("legacy console screen captured after blinking")
	if strings.Count(screen, "SpaceChat") != 1 {
		t.Fatal("login title is missing or duplicated")
	}
	labels, keyRow := 0, -1
	for row, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "房间口令") && !strings.Contains(line, "输入") {
			labels++
			keyRow = row
		}
	}
	if labels != 1 {
		t.Fatalf("expected one label in legacy console, got %d", labels)
	}
	sendConsoleText(t, process.ProcessId, "界面验收\r")
	time.Sleep(650 * time.Millisecond)
	sendConsoleText(t, process.ProcessId, "masked-room-key")
	time.Sleep(650 * time.Millisecond)
	screen = consoleScreen(t, process.ProcessId)
	if strings.Count(screen, "房间口令") != 1 || strings.Contains(screen, "masked-room-key") || !strings.Contains(screen, "界面验收") {
		t.Fatalf("focus or typing corrupted the login screen:\n%s", screen)
	}
	lines := strings.Split(screen, "\n")
	if keyRow >= len(lines) || !strings.Contains(lines[keyRow], "房间口令") {
		t.Fatal("typing moved the room key field to another row")
	}
}

type consoleKeyRecord struct {
	eventType uint16
	padding   uint16
	down      int32
	repeat    uint16
	key       uint16
	scan      uint16
	character uint16
	state     uint32
}

func sendConsoleText(t *testing.T, processID uint32, text string) {
	t.Helper()
	var records []consoleKeyRecord
	for _, character := range utf16.Encode([]rune(text)) {
		record := consoleKeyRecord{eventType: 1, down: 1, repeat: 1, character: character}
		if character == '\r' {
			record.key = 13
		}
		if character == '\t' {
			record.key = 9
		}
		records = append(records, record)
	}
	writeConsoleRecords(t, processID, records)
}

func sendConsoleKey(t *testing.T, processID uint32, key, character uint16, state uint32) {
	t.Helper()
	writeConsoleRecords(t, processID, []consoleKeyRecord{{eventType: 1, down: 1, repeat: 1, key: key, character: character, state: state}})
}

func writeConsoleRecords(t *testing.T, processID uint32, records []consoleKeyRecord) {
	t.Helper()
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	free := kernel.NewProc("FreeConsole")
	attach := kernel.NewProc("AttachConsole")
	free.Call()
	result, _, err := attach.Call(uintptr(processID))
	if result == 0 {
		t.Fatalf("attach console input: %v", err)
	}
	defer func() { free.Call(); attach.Call(uintptr(^uint32(0))) }()
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	var written uint32
	result, _, err = kernel.NewProc("WriteConsoleInputW").Call(input.Fd(), uintptr(unsafe.Pointer(&records[0])), uintptr(len(records)), uintptr(unsafe.Pointer(&written)))
	if result == 0 || int(written) != len(records) {
		t.Fatalf("write console input: %v", err)
	}
}
