# Changelog

All notable changes to mm will be documented in this file.

The format is based loosely on Keep a Changelog, and versions are recorded using repository tags.

## Unreleased

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
