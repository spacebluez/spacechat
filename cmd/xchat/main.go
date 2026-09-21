package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/client"
	"xchat/internal/clientconfig"
	"xchat/internal/terminal"
	"xchat/internal/tui"
	"xchat/internal/update"
	"xchat/internal/updateui"
)

var defaultServer = "ws://127.0.0.1:18081/ws"
var version = "dev"
var updatePublicKey = ""

var newModel = tui.NewWithClientInfo
var runTerminal = func(model tea.Model) error { return terminal.Run(model) }

type options struct {
	server      string
	serverSet   bool
	showVersion bool
	selfCheck   bool
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	set := flag.NewFlagSet("spacechat", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.Usage = func() {
		fmt.Fprintln(stderr, "Usage: spacechat [--server ws[s]://host/path] [--version]")
		fmt.Fprintln(stderr, "  --server string   WebSocket server address")
		fmt.Fprintln(stderr, "  --version         Show version")
	}
	server := set.String("server", "", "WebSocket server address")
	showVersion := set.Bool("version", false, "Show version")
	selfCheck := set.Bool("self-check", false, "")
	if err := set.Parse(args); err != nil {
		return options{}, err
	}
	if set.NArg() != 0 {
		return options{}, errors.New("unexpected positional arguments")
	}
	result := options{server: *server, showVersion: *showVersion, selfCheck: *selfCheck}
	set.Visit(func(item *flag.Flag) {
		if item.Name == "server" {
			result.serverSet = true
		}
	})
	return result, nil
}

func releaseMetadata(requireManaged bool) (update.Version, ed25519.PublicKey, bool, error) {
	if version == "dev" {
		if requireManaged {
			return update.Version{}, nil, false, errors.New("development build cannot pass release self-check")
		}
		return update.Version{}, nil, false, nil
	}
	parsed, err := update.ParseVersion(version)
	if err != nil {
		return update.Version{}, nil, false, fmt.Errorf("invalid embedded version: %w", err)
	}
	if updatePublicKey == "" {
		if requireManaged {
			return update.Version{}, nil, false, errors.New("release self-check requires an update public key")
		}
		return parsed, nil, false, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(updatePublicKey)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return update.Version{}, nil, false, errors.New("invalid embedded update public key")
	}
	return parsed, ed25519.PublicKey(decoded), true, nil
}

func launchUpdated(path string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	command := exec.Command(path, args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Start()
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	parsedOptions, err := parseOptions(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, "spacechat:", err)
		return 2
	}
	if parsedOptions.showVersion {
		fmt.Fprintln(stdout, "spacechat", version)
		return 0
	}
	current, publicKey, managed, err := releaseMetadata(parsedOptions.selfCheck)
	if err != nil {
		fmt.Fprintln(stderr, "spacechat:", err)
		return 1
	}
	if parsedOptions.selfCheck {
		return 0
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "spacechat:", err)
		return 1
	}
	layout, err := update.LayoutFor(runtime.GOOS, home, os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "spacechat:", err)
		return 1
	}
	address := parsedOptions.server
	if !parsedOptions.serverSet {
		config, loadError := clientconfig.Load(layout.Config, defaultServer)
		if loadError != nil {
			fmt.Fprintln(stderr, "spacechat:", loadError)
			return 2
		}
		address = config.Server
	}
	if err = client.ValidateAddress(address); err != nil {
		fmt.Fprintln(stderr, "spacechat:", err)
		return 2
	}
	info := client.Info{Version: version, OS: runtime.GOOS, Arch: runtime.GOARCH}
	console := updateui.New(stdin, stdout)
	remote := update.Remote{PublicKey: publicKey}
	installer := update.Installer{Layout: layout, Remote: remote, GOOS: runtime.GOOS, Progress: console.Progress}
	runner := update.Runner{
		Checker:   remote,
		Installer: installer,
		UI:        console,
		Launch: func(path string, childArgs []string) error {
			return launchUpdated(path, childArgs, stdin, stdout, stderr)
		},
	}
	if managed {
		if cleanupError := update.Cleanup(layout, current); cleanupError != nil {
			console.ShowError(fmt.Errorf("清理旧版本失败：%w", cleanupError))
		}
	}
	for {
		if managed {
			outcome, updateError := runner.Run(context.Background(), update.Request{
				Server: address, Current: current, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Args: args,
			})
			if updateError != nil {
				fmt.Fprintln(stderr, "spacechat:", updateError)
				return 1
			}
			if outcome == update.Relaunched || outcome == update.Exit {
				return 0
			}
		}
		model := newModel(address, info)
		runError := runTerminal(model)
		upgradeRequired := model.UpgradeRequired()
		model.Close()
		if runError != nil {
			fmt.Fprintln(stderr, "spacechat:", runError)
			return 1
		}
		if !upgradeRequired {
			return 0
		}
		if !managed {
			fmt.Fprintln(stderr, "spacechat: 服务端要求升级，但当前客户端未配置在线更新")
			return 1
		}
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
