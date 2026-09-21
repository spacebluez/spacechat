package update

import (
	"context"
	"errors"
	"testing"
)

type fakeChecker struct {
	check Check
	err   error
	calls int
}

func (checker *fakeChecker) Check(context.Context, string, Version, string, string) (Check, error) {
	checker.calls++
	return checker.check, checker.err
}

type fakeInstalling struct {
	results   []InstallResult
	errors    []error
	installs  int
	rollbacks int
}

func (installer *fakeInstalling) Install(context.Context, Check) (InstallResult, error) {
	index := installer.installs
	installer.installs++
	var result InstallResult
	if index < len(installer.results) {
		result = installer.results[index]
	}
	var err error
	if index < len(installer.errors) {
		err = installer.errors[index]
	}
	return result, err
}

func (installer *fakeInstalling) Rollback(InstallResult) error {
	installer.rollbacks++
	return nil
}

type fakeUI struct {
	actions []Action
	prompts []Prompt
	errors  []error
}

func (ui *fakeUI) Choose(prompt Prompt) Action {
	ui.prompts = append(ui.prompts, prompt)
	action := ui.actions[0]
	ui.actions = ui.actions[1:]
	return action
}
func (ui *fakeUI) Progress(int64, int64) {}
func (ui *fakeUI) ShowError(err error)   { ui.errors = append(ui.errors, err) }

func runnerRequest() Request {
	current, _ := ParseVersion("0.3.2")
	return Request{Server: "ws://chat/ws", Current: current, GOOS: "linux", GOARCH: "amd64", Args: []string{"--server", "ws://chat/ws"}}
}

func checkForDecision(decision Decision) Check {
	current, _ := ParseVersion("0.3.2")
	latest, _ := ParseVersion("0.4.0")
	minimum, _ := ParseVersion("0.3.2")
	if decision == DecisionRequired {
		current, _ = ParseVersion("0.3.1")
	}
	return Check{Current: current, Latest: latest, Minimum: minimum, Decision: decision}
}

func TestRunnerContinuesWithoutInstallForCurrentSkipAndCheckFailure(t *testing.T) {
	for name, checker := range map[string]*fakeChecker{
		"current":       {check: checkForDecision(DecisionCurrent)},
		"ahead":         {check: checkForDecision(DecisionAhead)},
		"check failure": {err: errors.New("server unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			installer := new(fakeInstalling)
			ui := &fakeUI{}
			runner := Runner{Checker: checker, Installer: installer, UI: ui}
			outcome, err := runner.Run(context.Background(), runnerRequest())
			if err != nil || outcome != Continue || installer.installs != 0 {
				t.Fatalf("outcome=%v err=%v installs=%d", outcome, err, installer.installs)
			}
			if name == "check failure" && len(ui.errors) != 1 {
				t.Fatal("check failure was not displayed")
			}
		})
	}

	checker := &fakeChecker{check: checkForDecision(DecisionOptional)}
	installer := new(fakeInstalling)
	ui := &fakeUI{actions: []Action{ActionSkip}}
	outcome, err := (Runner{Checker: checker, Installer: installer, UI: ui}).Run(context.Background(), runnerRequest())
	if err != nil || outcome != Continue || installer.installs != 0 {
		t.Fatalf("optional skip outcome=%v err=%v installs=%d", outcome, err, installer.installs)
	}
}

func TestRunnerOptionalFailureContinuesAndSuccessfulInstallRelaunches(t *testing.T) {
	request := runnerRequest()
	check := checkForDecision(DecisionOptional)
	installFailure := errors.New("download failed")
	ui := &fakeUI{actions: []Action{ActionUpdate}}
	installer := &fakeInstalling{errors: []error{installFailure}}
	outcome, err := (Runner{Checker: &fakeChecker{check: check}, Installer: installer, UI: ui}).Run(context.Background(), request)
	if err != nil || outcome != Continue || len(ui.errors) != 1 {
		t.Fatalf("optional failure outcome=%v err=%v errors=%v", outcome, err, ui.errors)
	}

	result := InstallResult{Path: "/managed/0.4.0/spacechat-client", Previous: request.Current, Target: check.Latest}
	ui = &fakeUI{actions: []Action{ActionUpdate}}
	installer = &fakeInstalling{results: []InstallResult{result}}
	launched := false
	runner := Runner{
		Checker:   &fakeChecker{check: check},
		Installer: installer,
		UI:        ui,
		Launch: func(path string, args []string) error {
			launched = path == result.Path && len(args) == 2 && args[0] == "--server"
			return nil
		},
	}
	outcome, err = runner.Run(context.Background(), request)
	if err != nil || outcome != Relaunched || !launched {
		t.Fatalf("successful update outcome=%v err=%v launched=%v", outcome, err, launched)
	}
}

func TestRunnerRequiredFailureRetriesOrExits(t *testing.T) {
	request := runnerRequest()
	request.Current, _ = ParseVersion("0.3.1")
	check := checkForDecision(DecisionRequired)
	result := InstallResult{Path: "/managed/0.4.0/spacechat-client", Previous: request.Current, Target: check.Latest}
	installer := &fakeInstalling{results: []InstallResult{{}, result}, errors: []error{errors.New("first failed"), nil}}
	ui := &fakeUI{actions: []Action{ActionUpdate, ActionRetry}}
	outcome, err := (Runner{Checker: &fakeChecker{check: check}, Installer: installer, UI: ui, Launch: func(string, []string) error { return nil }}).Run(context.Background(), request)
	if err != nil || outcome != Relaunched || installer.installs != 2 || len(ui.errors) != 1 || !ui.prompts[1].Retry {
		t.Fatalf("retry outcome=%v err=%v installs=%d prompts=%+v", outcome, err, installer.installs, ui.prompts)
	}

	installer = &fakeInstalling{errors: []error{errors.New("failed")}}
	ui = &fakeUI{actions: []Action{ActionUpdate, ActionExit}}
	outcome, err = (Runner{Checker: &fakeChecker{check: check}, Installer: installer, UI: ui}).Run(context.Background(), request)
	if err != nil || outcome != Exit || installer.installs != 1 {
		t.Fatalf("exit outcome=%v err=%v installs=%d", outcome, err, installer.installs)
	}
}

func TestRunnerRollsBackWhenNewProcessCannotStart(t *testing.T) {
	request := runnerRequest()
	check := checkForDecision(DecisionOptional)
	result := InstallResult{Path: "/managed/client", Previous: request.Current, Target: check.Latest}
	installer := &fakeInstalling{results: []InstallResult{result}}
	ui := &fakeUI{actions: []Action{ActionUpdate}}
	runner := Runner{
		Checker:   &fakeChecker{check: check},
		Installer: installer,
		UI:        ui,
		Launch:    func(string, []string) error { return errors.New("start failed") },
	}
	outcome, err := runner.Run(context.Background(), request)
	if err != nil || outcome != Continue || installer.rollbacks != 1 || len(ui.errors) != 1 {
		t.Fatalf("outcome=%v err=%v rollbacks=%d errors=%v", outcome, err, installer.rollbacks, ui.errors)
	}
}

func TestRunnerDoesNotRollbackAnotherUpdaterOnLaunchFailure(t *testing.T) {
	request := runnerRequest()
	check := checkForDecision(DecisionOptional)
	installer := &fakeInstalling{results: []InstallResult{{Path: "/managed/client", Previous: request.Current, Target: check.Latest, Shared: true}}}
	ui := &fakeUI{actions: []Action{ActionUpdate}}
	outcome, err := (Runner{
		Checker: &fakeChecker{check: check}, Installer: installer, UI: ui,
		Launch: func(string, []string) error { return errors.New("start failed") },
	}).Run(context.Background(), request)
	if err != nil || outcome != Continue || installer.rollbacks != 0 {
		t.Fatalf("outcome=%v err=%v rollbacks=%d", outcome, err, installer.rollbacks)
	}
}

func TestRunnerKnownIncompatibleCheckFailureRequiresRetryOrExit(t *testing.T) {
	request := runnerRequest()
	request.KnownIncompatible = true
	checker := &sequenceChecker{
		errors: []error{errors.New("update endpoint unavailable"), nil},
		checks: []Check{{}, checkForDecision(DecisionRequired)},
	}
	ui := &fakeUI{actions: []Action{ActionRetry, ActionExit}}
	outcome, err := (Runner{Checker: checker, Installer: new(fakeInstalling), UI: ui}).Run(context.Background(), request)
	if err != nil || outcome != Exit || checker.calls != 2 || len(ui.prompts) != 2 || !ui.prompts[0].Retry {
		t.Fatalf("outcome=%v err=%v calls=%d prompts=%+v", outcome, err, checker.calls, ui.prompts)
	}
}

type sequenceChecker struct {
	checks []Check
	errors []error
	calls  int
}

func (checker *sequenceChecker) Check(context.Context, string, Version, string, string) (Check, error) {
	index := checker.calls
	checker.calls++
	return checker.checks[index], checker.errors[index]
}
