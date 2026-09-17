package terminal

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/erikgeiser/coninput"
	"testing"
	"time"
)

func TestWindowsShiftEnterIsDistinct(t *testing.T) {
	plain := keyMessage(coninput.KeyEventRecord{KeyDown: true, VirtualKeyCode: coninput.VK_RETURN, Char: '\r'})
	shifted := keyMessage(coninput.KeyEventRecord{KeyDown: true, VirtualKeyCode: coninput.VK_RETURN, Char: '\r', ControlKeyState: coninput.SHIFT_PRESSED})
	if plain.Type != tea.KeyEnter || plain.Alt || shifted.Type != tea.KeyEnter || !shifted.Alt {
		t.Fatalf("enter encodings: %+v %+v", plain, shifted)
	}
}

func TestNativeBracketedPasteNeverSendsEnter(t *testing.T) {
	decoder := pasteDecoder{}
	var messages []tea.Msg
	send := func(message tea.Msg) { messages = append(messages, message) }
	for _, character := range "\x1b[200~one\r\ntwo\x1b[201~" {
		decoder.feed(coninput.KeyEventRecord{KeyDown: true, Char: character}, send)
	}
	if len(messages) != 1 {
		t.Fatalf("paste produced %d events", len(messages))
	}
	message := messages[0].(tea.KeyMsg)
	if !message.Paste || string(message.Runes) != "one\ntwo" {
		t.Fatalf("paste not preserved: %+v", message)
	}
}

func TestNativeRoomSwitchAndEscape(t *testing.T) {
	message := keyMessage(coninput.KeyEventRecord{KeyDown: true, VirtualKeyCode: coninput.VK_F2})
	if message.Type != tea.KeyF2 {
		t.Fatal("F2 not preserved")
	}
	decoder := pasteDecoder{}
	var messages []tea.Msg
	send := func(message tea.Msg) { messages = append(messages, message) }
	decoder.feed(coninput.KeyEventRecord{KeyDown: true, VirtualKeyCode: coninput.VK_ESCAPE, Char: '\x1b'}, send)
	decoder.flushPrefix(time.Now().Add(time.Second), send)
	if len(messages) != 1 || messages[0].(tea.KeyMsg).Type != tea.KeyEsc {
		t.Fatal("standalone Escape was swallowed")
	}
}
