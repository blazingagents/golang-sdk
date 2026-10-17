#!/usr/bin/env bash
# release-check.sh TAG NOTES_FILE [MAIN_REF]
#
# Checks that TAG (vX.Y.Z or vX.Y.Z-suffix) may be released from HEAD: it matches Version in
# client.go, HEAD is on MAIN_REF (default origin/main), and CHANGELOG.md has a non-empty
# "## X.Y.Z" entry. Writes that entry to NOTES_FILE and prints GitHub step outputs
# (version=, prerelease=) on stdout. Every failure names the fix.
set -euo pipefail

if [ $# -lt 2 ] || [ $# -gt 3 ]; then
  echo "usage: $0 TAG NOTES_FILE [MAIN_REF]" >&2
  exit 2
fi
tag=$1 notes=$2 main_ref=${3:-origin/main}

fail() {
  echo "::error::$*" >&2
  exit 1
}

semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$'
[[ $tag =~ $semver ]] || fail "tag $tag is not vX.Y.Z or vX.Y.Z-suffix"
version=${tag#v}

declared=$(sed -nE 's/^[[:space:]]*Version[[:space:]]*=[[:space:]]*"([^"]*)".*/\1/p' client.go)
[ -n "$declared" ] || fail "no Version constant found in client.go"
[ "$declared" = "$version" ] ||
  fail "tag $tag does not match Version \"$declared\" in client.go; set Version to \"$version\" in a PR before tagging"

git merge-base --is-ancestor HEAD "$main_ref" ||
  fail "commit $(git rev-parse --short HEAD) is not on $main_ref; tag a commit that has merged to main"

# The entry is every line after "## X.Y.Z" (optionally followed by a date) up to the next "## "
# heading, with blank lines trimmed from both ends.
awk -v v="$version" '
  /^## / { if (found) exit; split($0, w, " "); found = (w[2] == v); next }
  found { print }
' CHANGELOG.md | sed -e '/./,$!d' | tac | sed -e '/./,$!d' | tac >"$notes"
[ -s "$notes" ] || fail "CHANGELOG.md has no non-empty \"## $version\" entry; add one in a PR before tagging"

echo "version=$version"
if [ -n "${BASH_REMATCH[4]}" ]; then
  echo "prerelease=true"
else
  echo "prerelease=false"
fi
