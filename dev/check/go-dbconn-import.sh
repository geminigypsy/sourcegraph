#!/usr/bin/env bash

# This script ensures pkg/database/dbconn is only imported by services allowed to
# directly speak with the database.

echo "--- go dbconn import"

trap "echo ^^^ +++" ERR

set -euf -o pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"/../..

allowed_prefix=(
  github.com/sourcegraph/sourcegraph/cmd/frontend
  github.com/sourcegraph/sourcegraph/cmd/gitserver
  github.com/sourcegraph/sourcegraph/cmd/worker
  github.com/sourcegraph/sourcegraph/cmd/repo-updater
  github.com/sourcegraph/sourcegraph/cmd/migrator
  github.com/sourcegraph/sourcegraph/enterprise/cmd/frontend
  github.com/sourcegraph/sourcegraph/enterprise/cmd/worker
  github.com/sourcegraph/sourcegraph/enterprise/cmd/repo-updater
  github.com/sourcegraph/sourcegraph/enterprise/cmd/precise-code-intel-
  # Doesn't connect but uses db internals for use with sqlite
  github.com/sourcegraph/sourcegraph/cmd/symbols
  # Transitively depends on zoekt package which imports but does not use DB
  github.com/sourcegraph/sourcegraph/cmd/searcher
)

# Create regex ^(a|b|c)
allowed=$(printf "|%s" "${allowed_prefix[@]}")
allowed=$(printf "^(%s)" "${allowed:1}")

# shellcheck disable=SC2016
template='{{with $pkg := .}}{{ range $pkg.Deps }}{{ printf "%s imports %s\n" $pkg.ImportPath .}}{{end}}{{end}}'

if go list ./cmd/... ./enterprise/cmd/... |
  grep -Ev "$allowed" |
  xargs go list -f "$template" |
  grep "github.com/sourcegraph/sourcegraph/internal/database/dbconn"; then
  echo "Error: the above service(s) are not allowed to import internal/database/dbconn"
  echo "^^^ +++"
  exit 1
fi
