# Changelog

All notable changes to mm will be documented in this file.

The format is based loosely on Keep a Changelog, and versions are recorded using repository tags.

## [Unreleased]

### Added

- `mm archive`, a local archive of the channels you can read. `mm archive sync <dir>` fetches posts and attachments into a plain-file layout, incrementally: `state.json` holds a per-channel high-water mark and later runs ask the server only for what is newer. `--channels public` reaches public channels you have left, and channels archived while you were a member are included. `mm archive search <dir> <query>` searches it offline with channel, user, date and regex filters, printing a permalink for each match. `mm archive status <dir>` says what the archive holds.

  A sync enumerates a channel by paging it rather than through the `since` parameter. A `since` response is capped at about a thousand posts and ordered by update time, not creation time, so advancing a high-water mark to the newest post it returns steps over posts it never carried. `since` is still used as a cheap test of whether a channel has anything new. Deleted posts are not archived: paging omits them and `include_deleted` needs system admin.

- `mm archive sync` also archives direct and group messages, under `posts/direct/<username>.jsonl` and `posts/group/<usernames>.jsonl`. A channel keeps the team and name it was first archived under.

- `mm archive sync --workers <n>` reads several channels at once, and downloads attachments the same way. A sync spends nearly all of its time waiting on round trips, so this is most of what it costs. Four by default.

- `mm archive sync --since <date>` re-reads every channel back to that date and merges the result with what is on disk, to fill a gap an earlier sync left. An ordinary sync never looks behind its own high-water mark, so it cannot repair itself.

## [0.5.0] - 2026-05-26

### Changed

- All public error messages now start with the originating package name (e.g. `commands: listing bots: …`) to match the Mujin error-prefix convention; identifiers using `args`, `serverURL`, `postJSON`, `reactionJSON`, `unreadJSON`, `WebSocketUrl` are renamed to the Mujin acronym-casing rules (`arguments`, `serverUrl`, `postJson`, …, `WebSocketURL`). User-facing CLI output and command names are unchanged. (#13)

## [0.4.0] - 2026-05-26

### Added

- Per-profile read-only mode. Use `mm auth login --readonly` to create a read-only profile, or `mm auth set-readonly <profile> on|off` to toggle. When enabled, mm refuses any HTTP request that would mutate state on the Mattermost server (only GET/HEAD/OPTIONS and POST to `/search` endpoints are allowed). (#8)
- `mm auth list` and `mm auth status` now show the read-only flag. (#8)

### Changed

- `channel info` and other channel-arg commands now resolve channels by display name (in addition to the 26-char ID and slug), including names with hyphens or spaces that Mattermost search tokenization can't handle. (#9)
- Release automation: a new auto-release bot tags `X.Y.Z` and writes `CHANGELOG.md` on every push to `main`, sourcing entries from each merged PR's `## Changelog` block. A `Changelog Guard` workflow rejects PRs whose description's changelog block is still the template placeholder; bypass with the `skip-changelog` label. Major releases run via the `Major Release` workflow with a `MAJOR` confirmation input. (#12)

### Fixed

- `slash exec <dm-channel-id> "/<command>"` no longer fails with "Unable to find the existing team"; the team ID from the active profile is now passed to the server's `/commands/execute` endpoint, which it requires for DM/GM channels. (#10)
- `notify --channel <name>` no longer silently drops the filter when the active profile has no team set or when channel resolution fails; surfaces the resolution error instead. DM channel IDs (26-char) resolve without needing a team. (#11)
- `dm group <username> <message>` now rejects single-user lists client-side with a hint to use `mm dm send`; trims empty comma entries. (#11)
- `draft list` and `scheduled list` show the DM partner's username for direct-message rows via a new `channelDisplayLabel` helper, instead of an 8-char channel ID prefix. (#11)

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
