#!/usr/bin/env bash
#
# Self-test for scripts/work-marker-gate.sh: the gate must be able to FAIL.
# It fails on a marker the allowlist does not cover, on a stale or expired
# entry, and refuses to run (exit 2) on a malformed allowlist or a scan that
# read nothing. Markers are assembled at runtime, so this file never spells
# one, and every fixture lives in a throwaway repository under $TMPDIR.
#
# Copied from the work marker section of luthersystems/elps
# scripts/ci-gates-test.sh.
#
# Usage: scripts/work-marker-gate-test.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

pass=0
fail=0

ok() {
	pass=$((pass + 1))
	echo "PASS  $1"
}

bad() {
	fail=$((fail + 1))
	echo "FAIL  $1"
}

assert_exit() {
	local want="$1" desc="$2"
	shift 2
	local out rc
	out=$("$@" 2>&1)
	rc=$?
	if [ "$rc" -eq "$want" ]; then
		ok "$desc (exit $rc)"
	else
		bad "$desc — want exit $want, got $rc"
		echo "$out" | sed 's/^/        | /'
	fi
}

assert_contains() {
	local needle="$1" desc="$2"
	shift 2
	local out
	out=$("$@" 2>&1)
	if grep -qF -- "$needle" <<< "$out"; then
		ok "$desc"
	else
		bad "$desc — output did not contain '$needle'"
		echo "$out" | sed 's/^/        | /'
	fi
}

WM_SH="${SCRIPT_DIR}/work-marker-gate.sh"

if [ ! -x "$WM_SH" ]; then
	bad "scripts/work-marker-gate.sh is missing or not executable"
elif ! command -v git >/dev/null 2>&1; then
	echo "SKIP  git unavailable — work marker gate assertions not run"
else
	wm_tmp="$(mktemp -d)"
	wm_todo="TO""DO"
	wm_bug="B""UG"

	# wm_repo <dir> <allowlist-body> -- a repo with one clean file and the
	# given allowlist body. Fixture files are added by the caller.
	wm_repo() {
		mkdir -p "$1/scripts"
		git -C "$1" init -q
		printf 'package main\n\nfunc main() {}\n' >"$1/main.go"
		printf '# header\n%s' "$2" >"$1/scripts/work-markers.txt"
		git -C "$1" add -A
	}
	wm_add() {
		printf '%s\n' "$3" >"$1/$2"
		git -C "$1" add -A
	}
	wm_run() { env -C "$1" WORK_MARKER_TODAY="${2:-2026-06-01}" bash "$WM_SH"; }

	wm_repo "${wm_tmp}/clean" ""
	assert_exit 0 "work marker gate: a clean tree passes" wm_run "${wm_tmp}/clean"
	assert_contains "files scanned" "work marker gate: a clean result reports how much was scanned" \
		wm_run "${wm_tmp}/clean"

	wm_repo "${wm_tmp}/lower" ""
	wm_add "${wm_tmp}/lower" a.go "// $(printf '%s' "$wm_todo" | tr '[:upper:]' '[:lower:]') ${wm_todo}S _${wm_todo}_ DE${wm_bug}"
	assert_exit 0 "work marker gate: lower-case and embedded words do not match" wm_run "${wm_tmp}/lower"

	wm_repo "${wm_tmp}/dirty" ""
	wm_add "${wm_tmp}/dirty" a.go "// ${wm_todo}(#1): later"
	assert_exit 1 "work marker gate: an uncovered marker is CAUGHT" wm_run "${wm_tmp}/dirty"
	assert_contains "a.go:1:" "work marker gate: the hit names file and line" wm_run "${wm_tmp}/dirty"

	wm_repo "${wm_tmp}/covered" "a.go | ${wm_todo}(#1) | 2026-12-31 | #1 | deferred until the parser lands
"
	wm_add "${wm_tmp}/covered" a.go "// ${wm_todo}(#1): later"
	assert_exit 0 "work marker gate: a dated entry with an issue covers its line" wm_run "${wm_tmp}/covered"
	assert_exit 1 "work marker gate: an EXPIRED entry stops covering" wm_run "${wm_tmp}/covered" 2027-01-01
	assert_contains "ALLOW-EXPIRED" "work marker gate: an expired entry is named" \
		wm_run "${wm_tmp}/covered" 2027-01-01

	wm_repo "${wm_tmp}/second" "a.go | ${wm_todo}(#1) | 2026-12-31 | #1 | deferred until the parser lands
"
	wm_add "${wm_tmp}/second" a.go "// ${wm_todo}(#1): later, ${wm_bug} here"
	assert_exit 1 "work marker gate: a second marker on a covered line is still CAUGHT" wm_run "${wm_tmp}/second"

	wm_repo "${wm_tmp}/never" "a.go | \"${wm_bug}: | never | - | log prefix that tests match
"
	wm_add "${wm_tmp}/never" a.go "log.Print(\"${wm_bug}: x\")"
	assert_exit 0 "work marker gate: a never entry covers data that is not work" wm_run "${wm_tmp}/never"

	wm_repo "${wm_tmp}/stale" "a.go | ${wm_todo}(#1) | 2026-12-31 | #1 | deferred until the parser lands
"
	assert_exit 1 "work marker gate: an entry that covers nothing FAILS" wm_run "${wm_tmp}/stale"
	assert_contains "ALLOW-STALE" "work marker gate: a stale entry is named" wm_run "${wm_tmp}/stale"

	wm_case=0
	for wm_bad in \
		"a.go | ${wm_todo} | 2026-12-31 | - | dated entry with no issue" \
		"a.go | plain text | never | - | text without any marker" \
		"a.go | ${wm_todo} | 2026-02-30 | #1 | not a real calendar date" \
		"a.go | ${wm_todo} | never | - | short" \
		"a.go | ${wm_todo} | never | -" \
		"a/*.go | ${wm_todo} | never | - | a glob is not one exact path"; do
		wm_case=$((wm_case + 1))
		wm_repo "${wm_tmp}/bad-${wm_case}" "${wm_bad}
"
		wm_add "${wm_tmp}/bad-${wm_case}" a.go "// ${wm_todo}"
		assert_exit 2 "work marker gate: malformed entry ${wm_case} refuses to run" \
			wm_run "${wm_tmp}/bad-${wm_case}"
	done
	wm_repo "${wm_tmp}/dup" "a.go | ${wm_todo} | never | - | first entry for this text
a.go | ${wm_todo} | never | - | second entry for this text
"
	wm_add "${wm_tmp}/dup" a.go "// ${wm_todo}"
	assert_exit 2 "work marker gate: a duplicate entry refuses to run" wm_run "${wm_tmp}/dup"

	wm_repo "${wm_tmp}/noallow" ""
	git -C "${wm_tmp}/noallow" rm -q --cached scripts/work-markers.txt
	rm -f "${wm_tmp}/noallow/scripts/work-markers.txt"
	assert_exit 2 "work marker gate: a MISSING allowlist refuses to run" wm_run "${wm_tmp}/noallow"

	mkdir -p "${wm_tmp}/norepo"
	assert_exit 2 "work marker gate: OUTSIDE a repository refuses to report clean" wm_run "${wm_tmp}/norepo"

	wm_repo "${wm_tmp}/nocheckout" ""
	rm -f "${wm_tmp}/nocheckout/main.go"
	assert_exit 2 "work marker gate: an UNPOPULATED working tree refuses to report clean" \
		wm_run "${wm_tmp}/nocheckout"

	rm -rf "$wm_tmp"
fi

echo "work-marker-gate-test: ${pass} passed, ${fail} failed"
[ "$fail" -eq 0 ]
