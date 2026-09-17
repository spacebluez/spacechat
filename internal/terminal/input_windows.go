package terminal

import (
	"os"
	"strings"
	"time"
	"unicode/utf16"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erikgeiser/coninput"
	"golang.org/x/sys/windows"
)

func keyMessage(event coninput.KeyEventRecord) tea.KeyMsg {
	shift := event.ControlKeyState.Contains(coninput.SHIFT_PRESSED)
	control := event.ControlKeyState.Contains(coninput.LEFT_CTRL_PRESSED | coninput.RIGHT_CTRL_PRESSED)
	alt := event.ControlKeyState.Contains(coninput.LEFT_ALT_PRESSED | coninput.RIGHT_ALT_PRESSED)
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
	prefix  string
	content []rune
	active  bool
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

func Run(model tea.Model) error {
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
