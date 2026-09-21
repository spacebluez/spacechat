package update

import (
	"context"
	"errors"
	"fmt"
)

type Action uint8

const (
	ActionUpdate Action = iota
	ActionSkip
	ActionRetry
	ActionExit
)

type Prompt struct {
	Current  Version
	Latest   Version
	Notes    string
	Decision Decision
	Retry    bool
}

type UI interface {
	Choose(Prompt) Action
	Progress(received, total int64)
	ShowError(error)
}

type Checker interface {
	Check(context.Context, string, Version, string, string) (Check, error)
}

type Installing interface {
	Install(context.Context, Check) (InstallResult, error)
	Rollback(InstallResult) error
}

type Outcome uint8

const (
	Continue Outcome = iota
	Relaunched
	Exit
)

type Request struct {
	Server       string
	Current      Version
	GOOS, GOARCH string
	Args         []string
}

type Runner struct {
	Checker   Checker
	Installer Installing
	UI        UI
	Launch    func(path string, args []string) error
}

func (runner Runner) Run(ctx context.Context, request Request) (Outcome, error) {
	if runner.Checker == nil || runner.UI == nil {
		return Exit, errors.New("update runner is not configured")
	}
	check, err := runner.Checker.Check(ctx, request.Server, request.Current, request.GOOS, request.GOARCH)
	if err != nil {
		runner.UI.ShowError(fmt.Errorf("检查更新失败：%w", err))
		return Continue, nil
	}
	if check.Decision == DecisionCurrent || check.Decision == DecisionAhead {
		return Continue, nil
	}
	action := runner.UI.Choose(Prompt{Current: check.Current, Latest: check.Latest, Notes: check.Manifest.ReleaseNotes, Decision: check.Decision})
	if action == ActionSkip && check.Decision == DecisionOptional {
		return Continue, nil
	}
	if action == ActionExit {
		return Exit, nil
	}
	if action != ActionUpdate {
		return Exit, errors.New("invalid update action")
	}
	if runner.Installer == nil {
		return Exit, errors.New("update installer is not configured")
	}
	for {
		result, installError := runner.Installer.Install(ctx, check)
		if installError == nil {
			if runner.Launch == nil {
				installError = errors.New("updated client launcher is not configured")
			} else {
				installError = runner.Launch(result.Path, request.Args)
			}
			if installError == nil {
				return Relaunched, nil
			}
			if rollbackError := runner.Installer.Rollback(result); rollbackError != nil {
				return Exit, fmt.Errorf("start updated client: %v; rollback: %w", installError, rollbackError)
			}
		}
		runner.UI.ShowError(installError)
		if check.Decision == DecisionOptional {
			return Continue, nil
		}
		action = runner.UI.Choose(Prompt{Current: check.Current, Latest: check.Latest, Notes: check.Manifest.ReleaseNotes, Decision: check.Decision, Retry: true})
		if action == ActionExit {
			return Exit, nil
		}
		if action != ActionRetry {
			return Exit, errors.New("invalid required-update action")
		}
	}
}
