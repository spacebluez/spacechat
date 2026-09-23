package terminal

import (
	"os"
	"strings"
	"time"
	"unicode/utf16"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erikgeiser/coninput"
	"github.com/rivo/uniseg"
	"golang.org/x/sys/windows"
)

func consoleAmbiguousWidth() int {
	// Probe an inactive buffer with the console's font without touching the
	// visible screen. Code pages alone do not describe the font's cell widths.
	buffer, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateConsoleScreenBuffer").Call(
		windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, 0, 1, 0)
	if buffer == ^uintptr(0) {
		return 1
	}
	probe := os.NewFile(buffer, "console-width-probe")
	defer probe.Close()
	if _, err := probe.WriteString("\u00b7"); err != nil {
		return 1
	}
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(buffer), &info); err == nil && info.CursorPosition.X == 2 {
		return 2
	}
	return 1
}

func keyMessage(event coninput.KeyEventRecord) tea.KeyMsg {
	shift := event.ControlKeyState.Contains(coninput.SHIFT_PRESSED)
	control := event.ControlKeyState.Contains(coninput.LEFT_CTRL_PRESSED | coninput.RIGHT_CTRL_PRESSED)
	alt := event.ControlKeyState.Contains(coninput.LEFT_ALT_PRESSED | coninput.RIGHT_ALT_PRESSED)
	if !shift && !control && !alt {
		switch event.VirtualKeyCode {
		case coninput.VK_F2:
			return tea.KeyMsg{Type: tea.KeyF2}
		case coninput.VK_F3:
			return tea.KeyMsg{Type: tea.KeyF3}
		case coninput.VK_F4:
			return tea.KeyMsg{Type: tea.KeyF4}
		case coninput.VK_F5:
			return tea.KeyMsg{Type: tea.KeyF5}
		}
	}
	if event.Char == '\n' {
		return tea.KeyMsg{Type: tea.KeyCtrlJ}
	}
	if event.VirtualKeyCode == coninput.VK_RETURN {
		return tea.KeyMsg{Type: tea.KeyEnter, Alt: shift || alt}
	}
	if event.VirtualKeyCode == coninput.VK_TAB && shift {
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	}
	basic := map[uint16]tea.KeyType{0x25: tea.KeyLeft, 0x26: tea.KeyUp, 0x27: tea.KeyRight, 0x28: tea.KeyDown, 0x24: tea.KeyHome, 0x23: tea.KeyEnd, 0x21: tea.KeyPgUp, 0x22: tea.KeyPgDown, 0x2d: tea.KeyInsert, 0x2e: tea.KeyDelete, 0x08: tea.KeyBackspace}
	if kind, ok := basic[uint16(event.VirtualKeyCode)]; ok {
		if control {
			switch kind {
			case tea.KeyLeft:
				kind = tea.KeyCtrlLeft
			case tea.KeyRight:
				kind = tea.KeyCtrlRight
			case tea.KeyHome:
				kind = tea.KeyCtrlHome
			case tea.KeyEnd:
				kind = tea.KeyCtrlEnd
			}
		}
		if shift && kind == tea.KeyInsert {
			kind = tea.KeyCtrlV
		}
		return tea.KeyMsg{Type: kind, Alt: alt}
	}
	if event.Char > 0 && event.Char < 32 {
		return tea.KeyMsg{Type: tea.KeyType(event.Char), Alt: alt}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{event.Char}, Alt: alt && !control}
}

type pasteDecoder struct {
	prefix   string
	prefixAt time.Time
	content  []rune
	active   bool
}

func (decoder *pasteDecoder) feed(event coninput.KeyEventRecord, send func(tea.Msg)) {
	character := event.Char
	if character == 0 && (decoder.active || decoder.prefix != "") {
		return
	}
	if decoder.active {
		if len(decoder.content) < 2100 {
			decoder.content = append(decoder.content, character)
		} else {
			decoder.content = append(decoder.content[:2000], decoder.content[len(decoder.content)-5:]...)
			decoder.content = append(decoder.content, character)
		}
		if strings.HasSuffix(string(decoder.content), "\x1b[201~") {
			text := strings.TrimSuffix(string(decoder.content), "\x1b[201~")
			text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
			send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
			decoder.active = false
			decoder.content = nil
		}
		return
	}
	if decoder.prefix != "" || character == '\x1b' {
		decoder.prefix += string(character)
		decoder.prefixAt = time.Now()
		if decoder.prefix == "\x1b[200~" {
			decoder.prefix = ""
			decoder.active = true
			return
		}
		if strings.HasPrefix("\x1b[200~", decoder.prefix) {
			return
		}
		send(tea.KeyMsg{Type: tea.KeyEscape})
		for _, item := range decoder.prefix[1:] {
			send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{item}})
		}
		decoder.prefix = ""
		return
	}
	message := keyMessage(event)
	if message.Type == tea.KeyRunes && (character == 0 || character == '\ufffd') {
		return
	}
	send(message)
}

func (decoder *pasteDecoder) flushPrefix(now time.Time, send func(tea.Msg)) {
	if decoder.prefix == "" || now.Sub(decoder.prefixAt) < 50*time.Millisecond {
		return
	}
	prefix := decoder.prefix
	decoder.prefix = ""
	send(tea.KeyMsg{Type: tea.KeyEsc})
	for _, character := range prefix[1:] {
		send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}})
	}
}

func Run(model tea.Model) error {
	if launched, err := launchUnicodeHost(); launched || err != nil {
		return err
	}
	output := windows.Handle(os.Stdout.Fd())
	var originalOutput uint32
	if err := windows.GetConsoleMode(output, &originalOutput); err == nil {
		previousWidth := uniseg.EastAsianAmbiguousWidth
		uniseg.EastAsianAmbiguousWidth = consoleAmbiguousWidth()
		defer func() { uniseg.EastAsianAmbiguousWidth = previousWidth }()
		if err := windows.SetConsoleMode(output, originalOutput|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.DISABLE_NEWLINE_AUTO_RETURN); err != nil {
			return err
		}
		defer windows.SetConsoleMode(output, originalOutput)
	}
	handle := windows.Handle(os.Stdin.Fd())
	var original uint32
	if err := windows.GetConsoleMode(handle, &original); err != nil {
		_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
		return err
	}
	if err := windows.SetConsoleMode(handle, windows.ENABLE_WINDOW_INPUT|windows.ENABLE_EXTENDED_FLAGS); err != nil {
		return err
	}
	defer windows.SetConsoleMode(handle, original)
	program := tea.NewProgram(model, tea.WithInput(nil), tea.WithAltScreen())
	done := make(chan struct{})
	stopped := make(chan struct{})
	failures := make(chan error, 1)
	go func() {
		defer close(stopped)
		var high rune
		decoder := pasteDecoder{}
		for {
			select {
			case <-done:
				return
			default:
			}
			events, err := coninput.PeekNConsoleInputs(handle, 64)
			if err != nil {
				failures <- err
				program.Quit()
				return
			}
			if len(events) == 0 {
				decoder.flushPrefix(time.Now(), program.Send)
				time.Sleep(8 * time.Millisecond)
				continue
			}
			events, err = coninput.ReadNConsoleInputs(handle, uint32(len(events)))
			if err != nil {
				failures <- err
				program.Quit()
				return
			}
			for _, record := range events {
				switch event := record.Unwrap().(type) {
				case coninput.WindowBufferSizeEventRecord:
					program.Send(tea.WindowSizeMsg{Width: int(event.Size.X), Height: int(event.Size.Y)})
				case coninput.KeyEventRecord:
					if !event.KeyDown {
						continue
					}
					if event.Char >= 0xd800 && event.Char <= 0xdbff {
						high = event.Char
						continue
					}
					if high != 0 {
						if event.Char >= 0xdc00 && event.Char <= 0xdfff {
							event.Char = utf16.DecodeRune(high, event.Char)
						}
						high = 0
					}
					for repeat := 0; repeat < max(1, int(event.RepeatCount)); repeat++ {
						decoder.feed(event, program.Send)
					}
				}
			}
		}
	}()
	_, err := program.Run()
	close(done)
	<-stopped
	select {
	case failure := <-failures:
		if err == nil {
			err = failure
		}
	default:
	}
	return err
}
