# Contributing to mm

## Development setup

```bash
git clone https://github.com/ziyan/mm.git
cd mm
make build
make test
```

### Requirements

- Go 1.25+
- golangci-lint
- gotestsum (for coverage reports)

### Build commands

```bash
make build      # build the binary
make test       # run tests
make coverage   # run tests with coverage report
make lint       # run golangci-lint
make format     # run gofmt and goimports
make vendor     # tidy and vendor dependencies
make clean      # remove build artifacts
```

## Code conventions

### Naming

This project uses a modified naming convention that differs from standard Go in several ways. All contributors must follow these rules.

#### Acronym casing

When the **first alphabetical character is capitalized**, capitalize the entire acronym:

```go
// Correct
type SessionID string
func GetFTPID() string
var ReferenceURI string
func _CreateSessionID()
type URL string

// Wrong
type SessionId string
func GetFtpId() string
var ReferenceUri string
```

When the **first alphabetical character is lowercase**, capitalize only the first letter of the acronym:

```go
// Correct
sessionId := "abc"
referenceUri := "https://..."
getFtpId()
websocketUrl := "wss://..."
channelId := channel.Id

// Wrong
sessionID := "abc"
referenceURI := "https://..."
getFTPID()
websocketURL := "wss://..."
channelID := channel.Id
```

#### No abbreviations

Spell out names in full. Do not abbreviate. Package names are the only exception (keep them brief).

```go
// Correct
command, response, request, message, description, configuration, channel

// Wrong
cmd, resp, req, msg, desc, cfg, ch
```

#### Variable naming

- Errors should be named `err`
- Avoid single-letter variables except in very short closures or range loops where meaning is obvious
- Name things descriptively and consistently

```go
// Correct
apiClient, server, err := client.New()
currentUser, _, err := apiClient.GetMe(ctx, "")
channelId, err := resolveChannelId(ctx, apiClient, teamId, args[0])

// Wrong
c, s, err := client.New()
u, _, err := c.GetMe(ctx, "")
chID, err := resolveChannelID(ctx, c, tID, args[0])
```

#### Receiver names

Use `self` for struct method receivers:

```go
// Correct
func (self *Config) Save() error {
    data, err := json.MarshalIndent(self, "", "  ")
    ...
}

// Wrong
func (c *Config) Save() error { ... }
```

### Project structure

```
command/              # main entrypoint
internal/
  client/             # Mattermost API client wrapper
  commands/           # cobra command implementations
  config/             # configuration management
  logging/            # structured logging setup
  printer/            # output formatting (table, JSON, etc.)
vendor/               # vendored dependencies
```

### Adding a new command

1. Create a new file in `internal/commands/` (or add to an existing one)
2. Define the cobra command in an `init()` function and add it to `rootCommand` or a parent command
3. Name the run function `<noun><Verb>Run` (e.g., `channelListRun`, `postCreateRun`)
4. Follow the established patterns:
   - Use `client.New()` to get an API client
   - Use `resolveTeamId()` for team-scoped commands
   - Use `resolveChannelId()` for channel name resolution
   - Support `--json` output via `printer.JSONOutput`
   - Use `printer.PrintSuccess()`, `printer.PrintInfo()`, `printer.PrintTable()` for output
   - Return errors with `fmt.Errorf("context: %w", err)`

Example skeleton:

```go
func init() {
    myCommand := &cobra.Command{
        Use:   "my-action <arg>",
        Short: "Description of the action",
        Args:  cobra.ExactArgs(1),
        RunE:  myActionRun,
    }
    rootCommand.AddCommand(myCommand)
}

func myActionRun(command *cobra.Command, args []string) error {
    apiClient, server, err := client.New()
    if err != nil {
        return err
    }
    ctx := context.Background()

    // ... API calls ...

    if printer.JSONOutput {
        printer.PrintJSON(result)
        return nil
    }

    printer.PrintSuccess("Done")
    return nil
}
```

### Dependencies

All dependencies are vendored. After changing `go.mod`:

```bash
go mod tidy
go mod vendor
```

Always use `-mod=vendor` when building or testing.

### Testing

- Unit test pure functions (formatters, parsers, helpers)
- Test command tree structure and flag registration
- Command handler tests require a live Mattermost server and are not run in CI
- Run `make coverage` to generate an HTML coverage report

### Linting

The project uses golangci-lint with the following staticcheck rules suppressed (see `.golangci.yml`):

- **ST1000**: Package comments are not required
- **ST1003**: Acronym casing follows the Mujin convention, not Go standard
- **ST1006**: Receiver name `self` is intentional

The project also follows the Mujin coding standard, enforced locally by `mulint`. Configuration is in `mulint.yaml`. Conventions:

- Acronym casing: lowercase-first identifier capitalizes only the first letter (`postJson`, `serverUrl`); capitalized-first identifier capitalizes the full acronym (`WebSocketURL`, `SessionID`).
- No abbreviations: `arguments` not `args`, `command` not `cmd`, `response` not `resp`.
- No single-letter variables.
- Every error message starts with `<package>: `, e.g. `fmt.Errorf("commands: listing bots: %w", err)`.
- Every package declares `var log = logging.MustGetLogger("<pkg>") //nolint:unused`.

`make lint` runs `golangci-lint` and (if installed) `mulint`. Install mulint from `~/mujin/dev/mulint`.

### Commit messages

- Use imperative mood: "Add feature" not "Added feature"
- First line: concise summary (under 72 characters)
- Body: explain what and why, not how

## Releases

Releases are driven automatically by the **Auto Release** workflow on every push to `main`. The changelog source is each PR's description, not `CHANGELOG.md`.

Every PR description must include a `## Changelog` block — the template at `.github/pull_request_template.md` provides the scaffold. Pick exactly one section heading and write one bullet:

| Section heading | Bump |
|-----------------|------|
| `### Added` / `### Changed` / `### Removed` / `### Deprecated` | minor |
| `### Fixed` / `### Security` | patch |

The **Changelog Guard** workflow rejects any PR whose description's `## Changelog` block is still the template placeholder. Apply the `skip-changelog` label to bypass for CI tweaks, docs typos, or internal-only refactors.

After merge, the bot enumerates PRs since the last tag, extracts each `## Changelog` block, composes a new `## [X.Y.Z]` section in `CHANGELOG.md`, commits as `chore(release): X.Y.Z`, and pushes the tag — which triggers the existing **Release** workflow to build cross-platform binaries and publish a GitHub release with the same notes.

Major releases are manual: run the **Major Release** workflow from the Actions tab and type `MAJOR` into the confirmation input.
