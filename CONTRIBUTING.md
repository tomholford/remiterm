# Contributing

## Requirements

- [Go](https://go.dev/dl/) 1.26+ (see `go.mod`)
- [golangci-lint](https://golangci-lint.run/) on `PATH`

## Quality gate

One fail-fast sequence (fmt → lint → build → test):

```sh
./ops/gauntlet.sh
```

Run it after every code change. CI runs the same sequence on pull requests and `master`. Individual steps:

| Script | Purpose |
| --- | --- |
| `./ops/fmt.sh` | format (`golangci-lint fmt`) |
| `./ops/lint.sh` | `golangci-lint run` |
| `./ops/build.sh` | build → `bin/remiterm` (gitignored) |
| `./ops/test.sh` | `go test -count=1 ./...` |

Single package:

```sh
go test -count=1 -run TestName ./internal/api/
```

## Demo

`remiterm demo` starts an in-process fake RemiliaNET API and opens the TUI against it. No login, no network, no SQLite cache write. Useful for scroll, history, reply-chain, and preview work.

```sh
./bin/remiterm demo
./bin/remiterm demo --latency 400ms
./bin/remiterm demo --latency 200ms --pages 20 --page-size 50 --poll 1h
./bin/remiterm demo --verbose 2>demo.log
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--latency` | `300ms` | Artificial delay per API request |
| `--pages` | `100` | Synthetic history pages |
| `--page-size` | `100` | Messages per page (max 100) |
| `--poll` | `1h` | TUI live-poll interval (high = quiet while testing backscroll) |
| `--verbose` | off | Log each demo API request to stderr (corrupts the TUI unless redirected) |

Message 5 in the corpus has a sample PNG for `i`. Message 10 is a hub with children for the detail chain.

## Layout

```
cmd/remiterm/     main
internal/
  api/            /api/v1 HTTP client
  auth/           token store + OAuth PKCE
  cache/          local SQLite message/profile cache
  cmd/            cobra commands
  config/         viper defaults
  demo/           in-process fake API
  tui/            bubbletea chat UI
ops/              quality scripts (tracked)
bin/              build output (gitignored)
screenshots/      README images
```

Public transport is REST poll only. Browser WebSocket is intentionally unused.

## Release

Releases are cut by tag. GoReleaser (`.goreleaser.yaml`, `.github/workflows/release.yml`) cross-compiles, publishes GitHub Release archives + checksums, and pushes a Homebrew cask to `tomholford/homebrew-tap`.

```sh
git tag v0.1.1
git push origin v0.1.1
```

PRs that touch `.goreleaser.yaml` or the release workflow run `goreleaser release --snapshot --skip=publish`. Locally:

```sh
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```

The tap lives in a separate repo. The default `GITHUB_TOKEN` cannot write it; set repo secret `HOMEBREW_TAP_TOKEN` to a fine-grained PAT with Contents: Read and write on `tomholford/homebrew-tap`. GoReleaser still emits a deprecated `postflight` block; `ops/rewrite-cask-postflight.rb` converts it to `postflight_steps` and the release workflow pushes that to the tap. Prerelease tags (`v0.1.0-rc.1`) skip the tap upload.

## Inspiration

Charm-stack chat TUIs we skimmed for layout/keybinding patterns:

- [chatuino](https://github.com/julez-dev/chatuino) — mature Twitch IRC TUI (tabs, chat view, components)
- [chatterm](https://github.com/zigzter/chatterm) — Twitch chat, model split
- [TextTunnel](https://github.com/Abi-Liu/TextTunnel) — WS chat, `internal/client/{auth,http,ui}`
- [chit](https://github.com/edsammy/chit) — room chat keys (`R` reply, selection mode)
- [slk](https://github.com/gammons/slk) / [slack-shell](https://github.com/polidog/slack-shell) — Slack TUI depth (themes, threads)
- [marchat](https://github.com/Cod-e-Codes/marchat) — self-hosted group chat + Bubble Tea client

We intentionally stay smaller: single room, REST poll, no plugin system.
