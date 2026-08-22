# CLAUDE.md

Guidance for Claude Code (and other agents) working in this repository.

## Project

`remiterm` is a terminal client for [RemiliaNET](https://www.remilia.net) global chat. Bubble Tea v2 TUI, Cobra CLI, OAuth Authorization Code + PKCE login, REST poll only.

## Commands

Quality scripts live under `ops/` (the built binary is `bin/remiterm`, gitignored):

| Script | Purpose |
| --- | --- |
| `./ops/fmt.sh` | `golangci-lint fmt` (gofmt + gci) |
| `./ops/lint.sh` | `golangci-lint run` |
| `./ops/build.sh` | `go build -o bin/remiterm ./cmd/remiterm` |
| `./ops/test.sh` | `go test -count=1 ./...` |
| `./ops/gauntlet.sh` | **fmt → lint → build → test** (fail-fast) |

Requires [golangci-lint](https://golangci-lint.run/) on `PATH`.

### Agent loop (required)

After **every** code change, run:

```sh
./ops/gauntlet.sh
```

Do not stop at “it seems fine.” Iterate until the gauntlet exits 0. Fix format/lint failures in the same change set.

Single package tests:

```sh
go test -count=1 -run TestName ./internal/api/
```

## Layout

```
cmd/remiterm/     main
internal/
  api/            /api/v1 HTTP client
  auth/           token store + OAuth PKCE
  cache/          local SQLite message/profile cache
  cmd/            cobra commands
  config/         viper defaults
  tui/            bubbletea chat UI
ops/              quality scripts (tracked)
bin/              build output (gitignored)
```

Public transport is REST poll only. Browser WebSocket is intentionally unused.
