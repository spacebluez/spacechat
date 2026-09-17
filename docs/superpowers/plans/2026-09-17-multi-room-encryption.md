# Multi-room Encryption Implementation Plan

> **For agentic workers:** Use executing-plans to implement this plan task-by-task. No delegation requested.

**Goal:** Deliver isolated passphrase rooms, encrypted history, multiline Windows client, and a parallel deployment.

**Architecture:** Add an encrypted SQLite room repository and bind every session to one room. Reuse existing protocol ordering and cleanup controls; deploy to an independent systemd service.

**Tech Stack:** Go 1.26, SQLite, AES-GCM, HMAC-SHA256, Bubble Tea, Python 3, systemd.

## Global Constraints
- Never stop, restart, modify or clear the existing production service.
- No real deployment addresses or keys in tracked files.
- Enter sends; Shift+Enter inserts newline; paste must not send.
- 00:00 Asia/Shanghai clears all rooms; active users may recreate their room by sending.

## Task 1: Encrypted room storage
Files: internal/securestore/{store,room,key}.go and tests.
- [x] Add failing tests for room isolation, ciphertext persistence, reopen, wrong keys, tampering, and all-room cleanup.
- [x] Implement Open, Room, Health, ClearAll plus room-scoped Repository methods and restricted key loading.
- [x] Run go test ./internal/securestore.

## Task 2: Room-bound sessions
Files: internal/server/{server,session,cleanup,rooms}.go and room tests; cmd/xchat-server/main.go.
- [x] Add failing tests covering same nickname in different rooms, no cross-room history/broadcast, empty passphrases and cleanup/continued sending.
- [x] Add NewRooms factory, session repository and room identity, scoped users/presence, global atomic cleanup.
- [x] Require encryption key at new server startup and reject plaintext databases.
- [x] Run go test ./internal/server ./internal/client ./internal/securestore.

## Task 3: Multiline client
Files: internal/protocol, internal/tui, cmd/xchat, internal/terminal.
- [x] Add failing LF validation, multiline editor and native Shift+Enter tests.
- [x] Switch message input to textarea, preserve Enter sending, implement Windows modifier-aware input and paste handling.
- [x] Run Go tests and packaged executable ConPTY test.

## Task 4: Parallel deployment and delivery
Files: deploy/rooms, deploy/clear_history.py, scripts/build.ps1, README.md.
- [x] Add tests that new deployment uses only independent paths/units and clears all new rooms.
- [x] Implement isolated installer, key generation, timer, Python cleanup and versioned packaging.
- [x] Run full tests, vet, Linux race and shell/Python validation.
- [x] Record old process identity, deploy new instance only, verify new-room flows and old process identity unchanged.
- [ ] Commit/push new branch via relay and provide client/source artifacts with sanitized verification record.
