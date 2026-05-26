# Changelog

All notable changes to mm will be documented in this file.

The format is based loosely on Keep a Changelog, and versions are recorded using repository tags.

## [Unreleased]

### Changed

- CI: enable the auto-release bot. On every push to `main` the bot inspects `## [Unreleased]`; bullets under `Added`/`Changed`/`Removed`/`Deprecated` trigger a minor bump, bullets under `Fixed`/`Security` trigger a patch bump. Major releases run via the `Major Release` workflow with a `MAJOR` confirmation input. A `Changelog Guard` workflow blocks any PR that doesn't update `## [Unreleased]` (override with the `skip-changelog` label).

### Added

- Per-profile read-only mode. Use `mm auth login --readonly` to create a read-only profile, or `mm auth set-readonly <profile> on|off` to toggle. When enabled, mm refuses any HTTP request that would mutate state on the Mattermost server (only GET/HEAD/OPTIONS and POST to `/search` endpoints are allowed).
- `mm auth list` and `mm auth status` now show the read-only flag.

### Fixed

- `notify --channel <name>` no longer silently drops the filter when the active profile has no team set or when channel resolution fails; the command now surfaces the resolution error and exits. DM channel IDs (26-char) resolve without needing a team.
- `dm group <username> <message>` now errors clearly when fewer than 2 other usernames are given, instead of letting the server reject the request with a generic message. Mattermost requires at least 3 participants (including self) for a group channel.
- `draft list` and `scheduled list` now show the DM partner's username for direct-message rows instead of an 8-char channel ID prefix.

## [0.3.1] - 2026-04-01

Corrective patch release: adds the changelog entry missing from 0.3.0.

### Fixed

- `post list --threads` no longer shows duplicate replies; reply posts already expanded under their root are skipped from the main timeline.
- `post list --threads --user` now includes replies by the target user under other users' root posts.
- `post list` rejects incompatible flag combinations at startup: `--threads` + `--collapse-threads` (mutually exclusive), `--threads` + `--json` (unsupported), and `--count` + `--since` (ambiguous).

### Added

- `dm send` reads message body from stdin when the positional `[message]` argument is omitted, enabling pipe workflows.
- Comprehensive CLI-level tests for `post list` flag validation, thread reply deduplication, and `dm send` stdin acceptance.

## [0.2.0] - 2026-03-28

### Fixed

- `server info` now works with Mattermost 10.x by using `format=old` query parameter for the client config API.
- `dm list` no longer fails with an invalid API URL; switched to an endpoint that does not require a team ID.
- `dm list` now resolves DM partner usernames instead of showing raw user ID pairs.

### Added

- Integration test suite (44 tests) running against a real Mattermost instance via Docker Compose, covering auth, teams, channels, posts, threads, DMs, files, and more.
- `make test` now starts Mattermost via Docker Compose and runs both unit and integration tests.
- `make coverage` produces a combined coverage report including integration tests.
- GitHub Actions CI runs integration tests with Mattermost.

## [0.1.0] - 2026-03-24

### Added

- GitHub release publishing workflow for tagged builds.
- Release checksum publishing via `SHA256SUMS` manifest.
- Full-featured Mattermost CLI client with support for teams, channels, posts, threads, direct messages, drafts, scheduled posts, files, bookmarks, saved posts, users, preferences, emoji, webhooks, bots, groups, slash commands, notifications, and sessions.
- Authentication via personal access token with multi-server profile support.
- Real-time notification streaming via WebSocket.
- JSON output mode for all commands.
- Cross-platform builds for Linux, macOS, and Windows (amd64 and arm64).
- Version injection via git tags at build time.
- Channel name and ID resolution for flexible argument handling.
- Batch user info lookup to avoid N+1 API calls.
- Unread message listing with `--mentions` flag support.
