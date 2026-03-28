# Changelog

All notable changes to mm will be documented in this file.

The format is based loosely on Keep a Changelog, and versions are recorded using repository tags.

## Unreleased

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
