#!/usr/bin/env bash
#
# govulncheck, with one module's findings set aside.
#
# mm is a client. It depends on github.com/mattermost/mattermost/server/public
# for the API types, and that module requires the server itself, which is
# vendored and therefore scanned. Mattermost files its advisories against the
# server module, and they describe what a running server does: who may rename
# a group, which request bypasses a permission check. This program never runs
# any of it. govulncheck reaches them through package initialisation, which is
# reachability of a kind, but not one a client can be attacked through, and
# most of them have no fixed version to move to.
#
# Everything else fails this script, which is the point: the exclusion is one
# module, named here, rather than a threshold that quietly swallows the next
# real finding.
set -euo pipefail

EXCLUDED_MODULE='github.com/mattermost/mattermost/server/v8'
REPORT="$(mktemp)"
trap 'rm -f "$REPORT"' EXIT

export GOFLAGS=-mod=vendor

# The readable report first, for whoever reads the log. It exits non-zero when
# it finds anything, including what we go on to set aside, so it cannot decide
# the outcome on its own.
govulncheck ./... || true

govulncheck -format json ./... > "$REPORT"

python3 - "$REPORT" "$EXCLUDED_MODULE" <<'PYTHON'
import json, sys

report_path, excluded_module = sys.argv[1], sys.argv[2]
text = open(report_path).read()
decoder = json.JSONDecoder()
index = 0
findings = {}
while index < len(text):
    while index < len(text) and text[index] in ' \n\r\t':
        index += 1
    if index >= len(text):
        break
    message, index = decoder.raw_decode(text, index)
    finding = message.get('finding')
    if not isinstance(finding, dict):
        continue
    trace = finding.get('trace') or []
    if not trace or not isinstance(trace[0], dict):
        continue
    # The first frame is the vulnerable code itself. A finding with no symbol
    # there is a module this program requires but does not call.
    if not trace[0].get('function'):
        continue
    module = trace[0].get('module') or '?'
    findings.setdefault(finding.get('osv'), module)

remaining = sorted(osv for osv, module in findings.items() if module != excluded_module)
set_aside = len(findings) - len(remaining)

print()
print('%d vulnerabilities reachable, %d of them in %s and set aside.'
      % (len(findings), set_aside, excluded_module))
if remaining:
    print('Reachable and not excused:')
    for osv in remaining:
        print('  %s  https://pkg.go.dev/vuln/%s' % (osv, osv))
    sys.exit(1)
print('Nothing else reachable.')
PYTHON
