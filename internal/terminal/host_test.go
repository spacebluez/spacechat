package terminal

import "testing"

func TestUnicodeHostArgumentsLaunchInvokingClient(t *testing.T) {
	executable := `C:\Users\Alice\AppData\Local\SpaceChat\versions\0.4.0\spacechat-client.exe`
	arguments := unicodeHostArguments(executable, `C:\work`)
	if len(arguments) == 0 || arguments[len(arguments)-1] != executable {
		t.Fatalf("terminal arguments do not launch invoking client: %q", arguments)
	}
}
