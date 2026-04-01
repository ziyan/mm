# mm

A full-featured command-line client for [Mattermost](https://mattermost.com).

`mm` lets you interact with your Mattermost server entirely from the terminal: send messages, manage channels, stream real-time notifications, upload files, schedule posts, and much more.

## Features

- **Multi-server profiles** with token and password authentication
- **Real-time notifications** via WebSocket with event and channel filtering
- **Full post lifecycle**: create, edit, delete, pin, react, search, threads, reminders
- **Channel management**: join, leave, create, archive, read, favorite, notify settings
- **Direct messages and group chats**
- **Thread operations**: list, follow/unfollow, mark read/unread
- **File upload/download/search**
- **Drafts and scheduled posts** with flexible time parsing
- **Channel bookmarks, saved/flagged posts, preferences**
- **User operations**: status, avatar, typing indicator, autocomplete
- **Session and access token management**
- **Custom emoji, webhooks, bots, groups, slash commands, plugins**
- **JSON output** (`--json`) for scripting and piping
- **Shell completion** for bash, zsh, fish, and PowerShell
- **Static binary** with zero CGO dependencies

## Installation

### From source

```bash
git clone https://github.com/ziyan/mm.git
cd mm
make build
```

This produces a statically linked binary `./mm`. Copy it to your `$PATH`:

```bash
sudo cp mm /usr/local/bin/
```

### Requirements

- Go 1.25+

## Quick start

### Login

```bash
# Login with a personal access token (recommended)
mm auth login --url https://mattermost.example.com --token YOUR_TOKEN

# Login with username and password
mm auth login --url https://mattermost.example.com --user alice --password secret
```

### Set your active team

```bash
mm team list
mm team switch my-team
```

### Send a message

```bash
mm post create town-square "Hello from the CLI!"
```

### Read messages

```bash
mm post list town-square
mm post list town-square -n 50  # last 50 messages
```

### Stream real-time notifications

```bash
mm notify                          # all events
mm notify --event posted           # only new messages
mm notify --channel town-square    # only one channel
mm notify --json                   # JSON output for scripting
```

## Usage

### Global flags

```
--json               Output in JSON format
--token string       Override access token
--server string      Override server URL
-T, --team string    Override active team (by name)
-l, --log-level      Log level: DEBUG, INFO, WARNING, ERROR, CRITICAL (default: WARNING)
```

### Authentication

```bash
mm auth login --url URL --token TOKEN   # login with token
mm auth login --url URL -u USER -p PASS # login with password
mm auth status                          # show current profile
mm auth list                            # list all profiles
mm auth switch <profile>                # switch active profile
mm auth remove <profile>                # remove a profile
mm auth sessions                        # list active sessions
mm auth revoke-session <session-id>     # revoke a session
mm auth revoke-all                      # revoke all sessions
mm auth token-create <description>      # create personal access token
mm auth token-list                      # list your tokens
mm auth token-revoke <token-id>         # revoke a token
```

### Teams

```bash
mm team list                  # list your teams
mm team switch <name>         # set active team
mm team info [name]           # show team details
mm team members [name]        # list team members
mm team invite user@email.com # invite by email
```

### Channels

```bash
mm channel list                         # list joined channels
mm channel list --all                   # include unjoined channels
mm channel join <name>                  # join a channel
mm channel leave <name>                 # leave a channel
mm channel create <name>                # create public channel
mm channel create <name> --private      # create private channel
mm channel info <name>                  # show channel details
mm channel members <name>               # list members
mm channel archive <name>               # archive a channel
mm channel unread                       # list channels with unread messages
mm channel read <name>                  # mark as read
mm channel favorite <name>              # add to favorites
mm channel unfavorite <name>            # remove from favorites
mm channel notify <name>                # show notification settings
mm channel notify <name> --desktop all  # set desktop notifications
mm channel categories                   # list sidebar categories
```

### Posts / messages

```bash
mm post create <channel> <message>             # post a message
mm post create <channel> <msg> -f file.png     # post with attachment
mm post create <channel> <msg> --root-id ID    # reply in thread
mm post list <channel>                         # list recent messages (default: 20)
mm post list <channel> -n 50                   # last 50 messages
mm post list <channel> --since 24h             # posts in the last 24 hours
mm post list <channel> --since 2026-03-29      # posts since a date
mm post list <channel> --user alice            # only posts by alice
mm post list <channel> --threads               # inline thread replies
mm post list <channel> --threads --user alice  # alice's posts and replies
mm post list <channel> --collapse-threads      # roots only with reply counts
mm post list <channel> --full-id               # show full 26-char post IDs
# Note: --threads and --collapse-threads are mutually exclusive.
# --count (-n) and --since cannot be combined (--since returns all matching posts).
# --threads is not supported with --json; use --collapse-threads instead.
mm post thread <post-id>                       # view a thread
mm post reply <post-id> <message>              # reply to a thread
mm post edit <post-id> <new-message>           # edit a post
mm post delete <post-id>                       # delete a post
mm post pin <post-id>                          # pin a post
mm post unpin <post-id>                        # unpin a post
mm post react <post-id> thumbsup              # add reaction
mm post unreact <post-id> thumbsup            # remove reaction
mm post search <query>                         # search posts
mm post search <query> --or                    # OR search
mm post pinned <channel>                       # list pinned posts
mm post history <post-id>                      # show edit history
mm post remind <post-id> 1h                    # set reminder
```

### Direct messages

```bash
mm dm send <username> [message]                 # send a DM
echo "hello" | mm dm send <username>           # pipe message from stdin
mm dm read <username>                           # read DM history
mm dm read <username> -n 50                     # last 50 messages
mm dm list                                      # list DM conversations
mm dm group user1,user2 <message>               # send group message
```

### Threads

```bash
mm thread list                   # list your threads
mm thread list --unread          # only unread threads
mm thread view <thread-id>       # view a thread
mm thread follow <thread-id>     # follow a thread
mm thread unfollow <thread-id>   # unfollow a thread
mm thread read <thread-id>       # mark as read
mm thread unread <post-id>       # mark as unread
mm thread read-all               # mark all threads as read
```

### Drafts

```bash
mm draft list                              # list your drafts
mm draft create <channel> <message>        # create/update a draft
mm draft delete <channel>                  # delete a draft
```

### Scheduled posts

```bash
mm scheduled list                                       # list scheduled posts
mm scheduled create <channel> 1h30m "reminder message"  # schedule by duration
mm scheduled create <channel> 14:30 "afternoon msg"     # schedule by time today
mm scheduled create <channel> 2025-12-01T09:00 "msg"    # schedule by datetime
mm scheduled delete <id>                                # delete scheduled post
```

### Files

```bash
mm file upload <channel> file1.png file2.pdf   # upload files
mm file upload <channel> doc.pdf -m "check this out"
mm file download <file-id>                     # download to current dir
mm file download <file-id> output.pdf          # download to specific path
mm file download <file-id> -                   # download to stdout
mm file info <file-id>                         # show file info
mm file search <query>                         # search files
```

### Bookmarks

```bash
mm bookmark list <channel>                          # list bookmarks
mm bookmark add <channel> "Docs" https://example.com  # add bookmark
mm bookmark delete <channel> <bookmark-id>          # delete bookmark
```

### Saved / flagged posts

```bash
mm saved list                        # list saved posts
mm saved list --channel town-square  # filter by channel
mm saved add <post-id>               # save a post
mm saved remove <post-id>            # unsave a post
```

### User operations

```bash
mm user me                          # show your profile
mm user info <username>             # show user profile
mm user status                      # show your status
mm user status online               # set status (online/away/dnd/offline)
mm user status --message "In a meeting" --emoji calendar
mm user search <query>              # search users
mm user list                        # list users in team
mm user autocomplete <prefix>       # autocomplete usernames
mm user typing <channel>            # send typing indicator
mm user avatar get [username]       # download profile image
mm user avatar set <image-file>     # set profile image
mm user avatar reset                # reset to default
```

### Preferences

```bash
mm preference list                            # list all preferences
mm preference list display_settings           # list by category
mm preference set <category> <name> <value>   # set a preference
mm preference delete <category> <name>        # delete a preference
```

### Emoji

```bash
mm emoji list                        # list custom emoji
mm emoji create <name> <image-file>  # create custom emoji
mm emoji delete <name>               # delete custom emoji
mm emoji search <query>              # search emoji
```

### Webhooks

```bash
mm webhook list-incoming                              # list incoming webhooks
mm webhook list-outgoing                              # list outgoing webhooks
mm webhook create-incoming <channel> --display-name X # create incoming
mm webhook create-outgoing <channel> --display-name X --url https://...
mm webhook delete <id>                                # delete incoming
mm webhook delete <id> --outgoing                     # delete outgoing
```

### Bots

```bash
mm bot list                                  # list bots
mm bot create <username>                     # create a bot
mm bot create <username> --display-name Bot  # with display name
mm bot info <bot-id>                         # show bot details
mm bot disable <bot-id>                      # disable a bot
mm bot enable <bot-id>                       # enable a bot
```

### Groups

```bash
mm group list                       # list groups
mm group list --channel <name>      # groups in a channel
mm group members <group-id>         # list group members
mm group info <group-id>            # show group details
```

### Slash commands and plugins

```bash
mm slash exec <channel> /giphy cats  # execute a slash command
mm slash list                        # list custom commands
mm plugin list                       # list installed plugins
```

### Server

```bash
mm server ping    # check connectivity
mm server info    # show server version and details
```

### Shell completion

```bash
# Bash
mm completion bash > /etc/bash_completion.d/mm

# Zsh
mm completion zsh > "${fpath[1]}/_mm"

# Fish
mm completion fish > ~/.config/fish/completions/mm.fish
```

## JSON output

All commands support `--json` for machine-readable output:

```bash
mm channel list --json | jq '.[].name'
mm post list town-square --json | jq '.order[]'
mm user me --json | jq '.username'
```

## Multiple servers

```bash
mm auth login --url https://server1.com --token TOKEN1 --name work
mm auth login --url https://server2.com --token TOKEN2 --name personal
mm auth list
mm auth switch personal
```

## Configuration

Configuration is stored in `~/.config/mm/config.json` (or `$XDG_CONFIG_HOME/mm/config.json`). The file contains server profiles with authentication tokens.

## API coverage

### Supported

The following Mattermost REST API (v4) endpoint groups are fully supported:

| API group | CLI commands | Operations |
|---|---|---|
| **Users** | `user` | Get self, get by username, search, list, autocomplete, status get/set, custom status, profile image get/set/reset, typing indicator |
| **Teams** | `team` | List user teams, get by name, members, invite by email, switch active team |
| **Channels** | `channel` | List, join, leave, create (public/private), info, members, archive, unread, mark read, notification settings, favorite/unfavorite, sidebar categories |
| **Posts** | `post` | Create (with file attachments), list, get thread, reply, edit (patch), delete, pin/unpin, reactions add/remove, search, pinned posts, edit history, reminders |
| **Direct messages** | `dm` | Send DM, read DM history, list conversations, create group messages |
| **Threads** | `thread` | List user threads, view, follow/unfollow, mark read/unread, mark all read |
| **Files** | `file` | Upload (with post), download, info, search |
| **Drafts** | `draft` | List, create/update (upsert), delete |
| **Scheduled posts** | `scheduled` | List, create (with duration/datetime parsing), delete |
| **Channel bookmarks** | `bookmark` | List, add (link type), delete |
| **Saved/flagged posts** | `saved` | List (with channel filter), save, unsave |
| **Preferences** | `preference` | List (all or by category), set, delete |
| **Emoji** | `emoji` | List, create, delete, search/autocomplete |
| **Webhooks** | `webhook` | List/create/delete incoming, list/create/delete outgoing |
| **Bots** | `bot` | List, create, info, enable, disable |
| **Groups** | `group` | List (all or by channel), members, info |
| **Slash commands** | `slash` | Execute, list custom commands |
| **Plugins** | `plugin` | List installed (active/inactive) |
| **Sessions** | `auth` | List sessions, revoke session, revoke all |
| **Access tokens** | `auth` | Create, list, revoke personal access tokens |
| **Server** | `server` | Ping with status, client config/server info |
| **WebSocket** | `notify` | Real-time event streaming with event type and channel filters |
| **Authentication** | `auth` | Login (token or password), multi-profile management |

### Not supported

The following API groups are **not exposed** in the CLI. These are primarily server administration, enterprise, or system-level endpoints that are not relevant to day-to-day user workflows:

| API group | Reason |
|---|---|
| **Server configuration** (`GetConfig`, `UpdateConfig`, `PatchConfig`, `ReloadConfig`) | Admin-only, dangerous to expose in a user CLI |
| **LDAP** (sync, test, groups, certificates, migration) | Enterprise admin feature |
| **SAML** (certificates, metadata, migration) | Enterprise admin feature |
| **Compliance** (reports, exports) | Enterprise compliance officer feature |
| **Data retention** (policies, channels, teams) | Enterprise admin feature |
| **Elasticsearch / Bleve** (purge, test, indexing) | Admin search engine management |
| **Cluster** (status) | Admin high-availability management |
| **License** (upload, remove, get) | Admin licensing |
| **OAuth apps** (`CreateOAuthApp`, `GetOAuthApps`, `DeleteOAuthApp`) | Developer/admin app registration |
| **Outgoing OAuth connections** | Enterprise integration admin feature |
| **IP filters** | Enterprise network admin feature |
| **Access control policies** | Enterprise admin feature |
| **Content flagging / moderation** | Enterprise content moderation |
| **Exports / Imports** (bulk data) | Admin data migration |
| **Jobs** (create, get, cancel, download) | Admin background job management |
| **Logs** (`GetLogs`, `PostLog`) | Admin server log access |
| **Analytics** (`GetAnalyticsOld`) | Admin analytics/reporting |
| **Audits** (`GetAudits`) | Admin audit trail |
| **Plugin management** (install, upload, enable, disable, remove) | Admin plugin lifecycle |
| **Scheme management** (create, get, patch, delete) | Admin permission schemes |
| **Role management** (create, get, patch) | Admin role/permission management |
| **User admin** (create, delete, deactivate, promote/demote, update auth, MFA, password reset) | Admin user management |
| **Team admin** (create, delete, update, privacy, scheme) | Admin team management |
| **Channel admin** (convert, move, update scheme, privacy, moderation) | Admin channel management |
| **Remote clusters / shared channels** | Enterprise federation features |
| **Cloud** (customer, products, subscription, invoices) | Mattermost Cloud admin |
| **Notices** (product notices) | System notice management |
| **Marketplace** (list, install marketplace plugins) | Admin plugin marketplace |
| **Integrity check** (`CheckIntegrity`) | Admin data integrity |
| **Brand image** (upload, get, delete) | Admin branding |
| **Terms of service** | Admin legal/compliance |
| **Reports** (user reporting) | Admin reporting |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding conventions, and guidelines.

## License

MIT
