package uninstall

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func checkRunning(p plan) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == uint32(os.Getpid()) {
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
		if err == nil && within(p.layout.Root, windows.UTF16ToString(buffer[:size])) {
			return fmt.Errorf("close all SpaceChat clients and bundled terminal windows before uninstalling (process %d)", entry.ProcessID)
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	return nil
}

func cleanPath(p plan) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return cleanUserPath(key, p.layout.Bin)
}

func cleanUserPath(key registry.Key, bin string) error {
	value, kind, err := key.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	entries := strings.Split(value, ";")
	kept := make([]string, 0, len(entries))
	for _, entry := range entries {
		normalized := filepath.Clean(strings.Trim(strings.TrimSpace(entry), `"`))
		if !strings.EqualFold(normalized, filepath.Clean(bin)) {
			kept = append(kept, entry)
		}
	}
	cleaned := strings.Join(kept, ";")
	if cleaned == value {
		return nil
	}
	if kind == registry.EXPAND_SZ {
		return key.SetExpandStringValue("Path", cleaned)
	}
	return key.SetStringValue("Path", cleaned)
}

func removeLauncher(p plan) (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	if !strings.EqualFold(filepath.Clean(executable), p.launcher) {
		if err := removeFile(p.launcher); err != nil {
			return false, err
		}
		return false, removeEmptyDirectory(p.layout.Bin)
	}
	// Give the running image a unique name so delayed cleanup cannot delete a new installation.
	temporary, err := os.CreateTemp(p.layout.Bin, ".spacechat-uninstall-*.exe")
	if err != nil {
		return false, err
	}
	if err := temporary.Close(); err != nil {
		os.Remove(temporary.Name())
		return false, err
	}
	if err := os.Rename(p.launcher, temporary.Name()); err != nil {
		os.Remove(temporary.Name())
		return false, err
	}
	started := false
	defer func() {
		if !started {
			os.Rename(temporary.Name(), p.launcher)
		}
	}()
	// Windows locks the running image. A hidden helper removes it after this process exits.
	log, err := os.CreateTemp("", "spacechat-uninstall-*.log")
	if err != nil {
		return false, err
	}
	log.Close()
	script := fmt.Sprintf(launcherCleanup, os.Getpid(), psQuote(temporary.Name()), psQuote(p.layout.Bin), psQuote(p.layout.Root), psQuote(log.Name()))
	encoded := utf16.Encode([]rune(script))
	data := make([]byte, len(encoded)*2)
	for i, unit := range encoded {
		binary.LittleEndian.PutUint16(data[2*i:], unit)
	}
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	command := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	command.Dir = os.TempDir()
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := command.Start(); err != nil {
		os.Remove(log.Name())
		return false, err
	}
	started = true
	return true, command.Process.Release()
}

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

const launcherCleanup = `
$ErrorActionPreference = 'Stop'
$parent = Get-Process -Id %d -ErrorAction SilentlyContinue
$launcher = %s
$bin = %s
$root = %s
$log = %s
try {
    if ($null -ne $parent) { $parent.WaitForExit() }
    foreach ($path in @($launcher, $bin, $root)) {
        $current = $path
        while (-not [string]::IsNullOrEmpty($current)) {
            if (Test-Path -LiteralPath $current) {
                $item = Get-Item -LiteralPath $current -Force
                if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) {
                    throw "Refusing reparse point: $current"
                }
            }
            $current = [IO.Path]::GetDirectoryName($current)
        }
    }
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        try { [IO.File]::Delete($launcher); break }
        catch { if ($attempt -eq 29) { throw }; Start-Sleep -Milliseconds 200 }
    }
    foreach ($directory in @($bin, $root)) {
        if ((Test-Path -LiteralPath $directory -PathType Container) -and
            @(Get-ChildItem -LiteralPath $directory -Force).Count -eq 0) {
            [IO.Directory]::Delete($directory, $false)
        }
    }
    [IO.File]::Delete($log)
} catch {
    [IO.File]::WriteAllText($log, ($_ | Out-String))
    exit 1
}
`
