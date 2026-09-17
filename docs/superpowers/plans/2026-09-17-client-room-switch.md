# Client Room Switch Implementation Plan

**Goal:** Switch rooms inside the client without losing the old room on failure.

**Architecture:** A modal owns editable fields and an independent candidate model/connection. Only a fully synchronized candidate replaces the active model. Source-tagged events isolate old and new sessions.

**Constraints:** Client-only change on the existing feature branch. Do not deploy or restart any server. Do not commit real addresses, keys or binaries. No new dependencies.

## Task 1: Switching state and modal
Files: internal/tui/room_switch.go, room_switch_view.go, model.go, view.go, room_switch_test.go.
- Add failing tests for modal cancellation, draft confirmation, duplicate submission, no-op same room, pending sends, candidate failure/success and stale events.
- Implement isolated candidate lifecycle, 15-second connection deadline, compact masked input view and F2 shortcut.
- Run focused TUI tests and full Go regression.

## Task 2: Network and native keyboard verification
Files: internal/tui/room_switch_network_test.go, internal/terminal/input_windows.go and tests, internal/tui/paste.go.
- Verify real encrypted-server target success, name conflicts, original room retention, timeout, cancellation and source-tagged clipboard handling.
- Verify F2 and Esc in native Windows events; preserve Shift+Enter and paste behavior.
- Run real packaged client ConPTY and Linux race regression on isolated temporary test data only.

## Task 3: Client delivery
Files: README.md, scripts/build-client.ps1, docs/verification-room-switch-2026-09-17.md.
- Document the workflow and client-only build command.
- Produce versioned 0.3.2 client and source packages without changing existing executables or any server.
- Report verification evidence and artifact locations.
