package updateui

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"xchat/internal/update"
)

type Console struct {
	reader *bufio.Reader
	output io.Writer
}

func New(input io.Reader, output io.Writer) *Console {
	return &Console{reader: bufio.NewReader(input), output: output}
}

func (console *Console) Choose(prompt update.Prompt) update.Action {
	for {
		if prompt.Retry {
			fmt.Fprintf(console.output, "更新失败。请选择 Retry [r] / Exit [e]：")
		} else if prompt.Decision == update.DecisionRequired {
			fmt.Fprintf(console.output, "SpaceChat %s 必须更新到 %s 后才能继续。\n%s\n请选择 Update [u] / Exit [e]：", prompt.Current, prompt.Latest, prompt.Notes)
		} else {
			fmt.Fprintf(console.output, "发现 SpaceChat %s（当前 %s）。\n%s\n请选择 Update [u] / Skip [s]：", prompt.Latest, prompt.Current, prompt.Notes)
		}
		line, err := console.reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if prompt.Decision == update.DecisionOptional && !prompt.Retry {
				return update.ActionSkip
			}
			return update.ActionExit
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		switch {
		case prompt.Retry && (choice == "r" || choice == "retry"):
			return update.ActionRetry
		case !prompt.Retry && (choice == "u" || choice == "update"):
			return update.ActionUpdate
		case !prompt.Retry && prompt.Decision == update.DecisionOptional && (choice == "s" || choice == "skip"):
			return update.ActionSkip
		case (prompt.Retry || prompt.Decision == update.DecisionRequired) && (choice == "e" || choice == "exit"):
			return update.ActionExit
		default:
			fmt.Fprintln(console.output, "无效选择，请重新输入。")
		}
	}
}

func (console *Console) Progress(received, total int64) {
	if total > 0 {
		fmt.Fprintf(console.output, "\r下载进度 %d/%d 字节", received, total)
		if received >= total {
			fmt.Fprintln(console.output)
		}
		return
	}
	fmt.Fprintf(console.output, "\r已下载 %d 字节", received)
}

func (console *Console) ShowError(err error) {
	if err != nil {
		fmt.Fprintln(console.output, "更新提示：", err)
	}
}
