package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A double-click owns its console; shells and pseudoterminals keep their host.
func launchUnicodeHost() (bool, error) {
	if len(os.Args) != 1 || os.Getenv("WT_SESSION") != "" {
		return false, nil
	}
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	window, _, _ := kernel.NewProc("GetConsoleWindow").Call()
	visible, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowVisible").Call(window)
	if window == 0 || visible == 0 {
		return false, nil
	}
	var processes [2]uint32
	count, _, _ := kernel.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&processes[0])), uintptr(len(processes)))
	if count != 1 {
		return false, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	host := filepath.Join(filepath.Dir(executable), "terminal", "WindowsTerminal.exe")
	if info, err := os.Stat(host); err != nil || info.IsDir() {
		return false, nil
	}
	directory, err := os.Getwd()
	if err != nil {
		return false, err
	}
	command := exec.Command(host, "-w", "new", "new-tab", "--profile", "XChat", "--startingDirectory", directory)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := command.Start(); err != nil {
		return false, err
	}
	return true, command.Process.Release()
}
