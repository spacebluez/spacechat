package tui

import (
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestDeployedWindowsClientTLS(t *testing.T) {
	executable, address, room := os.Getenv("XCHAT_EXE"), os.Getenv("XCHAT_GUI_DEPLOY_ADDRESS"), os.Getenv("XCHAT_SMOKE_KEY")
	if executable == "" || address == "" || room == "" {
		t.Skip("set XCHAT_EXE, XCHAT_GUI_DEPLOY_ADDRESS and a dedicated XCHAT_SMOKE_KEY")
	}
	arguments := windows.EscapeArg(executable)
	if address != "default" {
		arguments += " --server " + windows.EscapeArg(address)
	}
	command, err := windows.UTF16PtrFromString(arguments)
	if err != nil {
		t.Fatal(err)
	}
	startup := windows.StartupInfo{}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESHOWWINDOW
	startup.ShowWindow = 0
	var process windows.ProcessInformation
	if err := windows.CreateProcess(nil, command, nil, nil, false, windows.CREATE_NEW_CONSOLE, nil, nil, &startup, &process); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(process.Process)
	defer windows.CloseHandle(process.Thread)
	defer windows.TerminateProcess(process.Process, 1)
	time.Sleep(1500 * time.Millisecond)
	eventually(t, "client login did not render", func() bool { return strings.Contains(consoleScreen(t, process.ProcessId), "房间口令") })
	sendConsoleText(t, process.ProcessId, "exe-"+time.Now().Format("150405")+"\r")
	time.Sleep(150 * time.Millisecond)
	sendConsoleText(t, process.ProcessId, room+"\r")
	eventually(t, "compiled client did not connect with built-in CA", func() bool {
		screen := consoleScreen(t, process.ProcessId)
		return strings.Contains(screen, "已连接") && strings.Contains(screen, "TLS")
	})
	query, body := os.Getenv("XCHAT_GUI_KAOMOJI_QUERY"), os.Getenv("XCHAT_GUI_KAOMOJI_TEXT")
	if query != "" && body != "" {
		sendConsoleKey(t, process.ProcessId, 0x72, 0, 0)
		eventually(t, "compiled client kaomoji picker did not open", func() bool { return strings.Contains(consoleScreen(t, process.ProcessId), "颜文字") })
		sendConsoleText(t, process.ProcessId, query)
		eventually(t, "server-configured expression was not displayed", func() bool { return strings.Contains(consoleScreen(t, process.ProcessId), body) })
		sendConsoleText(t, process.ProcessId, "\x13")
	} else {
		body = "executable TLS check"
		sendConsoleText(t, process.ProcessId, body+"\r")
	}
	eventually(t, "compiled client message was not confirmed", func() bool {
		screen := consoleScreen(t, process.ProcessId)
		return strings.Contains(screen, body) && !strings.Contains(screen, "颜文字 ·") && !strings.Contains(screen, "待确认")
	})
	sendConsoleKey(t, process.ProcessId, 0x74, 0, 0)
	eventually(t, "compiled client recall picker did not open", func() bool { return strings.Contains(consoleScreen(t, process.ProcessId), "撤回消息") })
	sendConsoleText(t, process.ProcessId, "\r\r")
	eventually(t, "compiled client recall did not complete", func() bool { return strings.Contains(consoleScreen(t, process.ProcessId), "消息已撤回") })
	sendConsoleText(t, process.ProcessId, "\x03")
	if status, err := windows.WaitForSingleObject(process.Process, 5000); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("compiled client did not exit", err)
	}
	t.Log("packaged Windows client connected to deployed WSS server using embedded CA, sent and recalled a message")
}
