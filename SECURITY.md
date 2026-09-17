# Reporting a security problem

Please do not open a public issue for anything exploitable.

Report it through GitHub's private vulnerability reporting, on the
[Security tab](https://github.com/ziyan/mm/security/advisories/new) of this
repository. It stays private to the maintainers until an advisory is
published, and it gives us somewhere to talk that is not a public issue
thread.

Tell us what you did, what happened, and what you expected. A transcript, a
server response, or an archive directory that reproduces it is worth more than
a description of one.

## What is in scope

This program, as a user runs it against a Mattermost server. In particular:

- **The access token.** It is written to the configuration file, passed as
  `Authorization` on every request, and must not reach anywhere else: not a
  log line, not an error message, not a request to another host.
- **Read-only profiles.** A profile marked read-only, at `mm auth login
  --readonly` or with `mm auth set-readonly`, refuses anything that is not a
  safe method on a safe path. A way to make one write is a bug in the guard,
  not a feature of the command that found it.
- **`mm archive`.** It writes a file per channel, named after data the server
  supplies. A server, or a hand-edited `state.json`, that can make it write
  outside the archive directory is in scope, as is one archived channel
  overwriting another's file.
- **Anything the server sends being treated as more than data.** Post content,
  channel names, usernames and file names all arrive from other people.

## What is not

- The Mattermost server itself. This is a client; it vendors
  `mattermost/server/public` for the API types, which requires the server
  module, so a scan of this repository reports advisories filed against the
  server. It never runs that code.
  `.github/scripts/check-vulnerabilities.bash` sets that module aside by name
  and fails on anything else.
- What a token is allowed to do. The server decides that. A token that can
  read a channel is meant to read it, and `--channels public` reaching a
  public channel you have left is the API working as documented.
- The archive directory's own permissions. It holds everything the account
  could read, in plain files, and is as private as the directory you put it
  in.
- Denial of service by volume against a server you are entitled to read.
  Algorithmic problems in this program are in scope.

## What already runs

Every push and every week: `gitleaks` over the whole history, CodeQL, and
`govulncheck` against the current database. Dependabot proposes updates
weekly, and every GitHub Actions step is pinned to a commit. Findings and the
reasons any were set aside are on the Security tab.
