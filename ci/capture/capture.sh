#!/bin/sh
# Everything ci/capture does after the install, in a file of its own so go test can run it against the fake GitHub
# with a stand-in loupe on PATH. action.yml passes the inputs as LOUPE_CI_* variables.
set -eu

pr=$LOUPE_CI_PULL_REQUEST
source=$LOUPE_CI_SOURCE
repo=$GITHUB_REPOSITORY
poll=${LOUPE_CI_POLL_SECONDS:-15}
grace=${LOUPE_CI_GRACE_SECONDS:-60}

# The number reaches URLs and GITHUB_OUTPUT, where a newline would set another output.
case "$pr" in
'' | *[!0-9]*)
	echo "::error::pull_request '$pr' is not a pull request number" >&2
	exit 1
	;;
esac
case "$LOUPE_CI_WAIT_FOR_CHECKS" in
'' | *[!0-9]*)
	echo "::error::wait_for_checks '$LOUPE_CI_WAIT_FOR_CHECKS' is not a whole number of minutes" >&2
	exit 1
	;;
esac

automatic=false
if [ "$GITHUB_EVENT_NAME" = pull_request ]; then
	case "${LOUPE_CI_EVENT_ACTION:-}" in
	opened | ready_for_review | synchronize) automatic=true ;;
	esac
fi

api() {
	curl --fail-with-body -sS -H "Authorization: Bearer $GITHUB_TOKEN" -H 'Accept: application/vnd.github+json' \
		"$GITHUB_API_URL/$1"
}

# Every page of a list endpoint, as one JSON array of the items under $2. GitHub caps a page at 100. Pages collect in a
# file, because a busy commit's runs outgrow the 128 KiB Linux allows one argument. Every step is checked by hand:
# callers run this inside an if, where set -e does not apply.
api_all() {
	pages=$(mktemp) || return 1
	page=1
	while :; do
		if ! {
			body=$(api "$1&per_page=100&page=$page") &&
				printf '%s' "$body" | jq -c --arg key "$2" '.[$key][]' >>"$pages" &&
				listed=$(printf '%s' "$body" | jq -e '.total_count') &&
				new=$(printf '%s' "$body" | jq -e --arg key "$2" '.[$key] | length') &&
				got=$(wc -l <"$pages")
		}; then
			rm -f "$pages"
			return 1
		fi
		if [ "$got" -ge "$listed" ] || [ "$new" -eq 0 ]; then
			break
		fi
		page=$((page + 1))
	done
	if ! jq -c -s '.' "$pages"; then
		rm -f "$pages"
		return 1
	fi
	rm -f "$pages"
}

skip() {
	echo "Skipping the review of $repo#$pr: $1 ($2)."
	printf 'loupe skipped the review of %s#%s (%s): %s\n' "$repo" "$pr" "$1" "$2" >>"$GITHUB_STEP_SUMMARY"
	{
		echo "skip=true"
		echo "reason=$1"
	} >>"$GITHUB_OUTPUT"
	exit 0
}

# A fork's pull_request event gets a read-only token whatever the job asks for, so no round could publish. A deleted
# fork's head.repo is null.
pull=$(api "repos/$repo/pulls/$pr")
head_repo=$(printf '%s' "$pull" | jq -r '.head.repo.full_name // ""')
if [ "$head_repo" != "$repo" ]; then
	skip fork "The pull request's head is not in $repo."
fi

LOUPE_HOME=$RUNNER_TEMP/loupe-home
export LOUPE_HOME
header=$(printf 'x-access-token:%s' "$GITHUB_TOKEN" | base64 | tr -d '\n')
# --json puts a refusal on stdout, so it is read from there.
if ! result=$(GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=http.https://github.com/.extraheader \
	GIT_CONFIG_VALUE_0="AUTHORIZATION: basic $header" \
	loupe capture "https://github.com/$repo/pull/$pr" --source "$source" --json); then
	message=$(printf '%s' "$result" | jq -r '.error.message // empty' 2>/dev/null || true)
	echo "::error::loupe capture refused: ${message:-$result}" >&2
	printf 'error: %s\n' "${message:-$result}" >&2
	fix=$(printf '%s' "$result" | jq -r '.error.fix // empty' 2>/dev/null || true)
	if [ -n "$fix" ]; then
		printf 'fix: %s\n' "$fix" >&2
	fi
	exit 1
fi
run=$(printf '%s' "$result" | jq -er .run)
head=$(printf '%s' "$result" | jq -er '.target.headSha | select(test("^[0-9a-f]{40}$"))')
previous_head=$(printf '%s' "$result" | jq -r '.previous.head // ""')

if [ "$automatic" = true ] && [ "$previous_head" = "$head" ]; then
	skip already-reviewed "Source $source already reviewed $head. Ask for a round to review it again."
fi

# This workflow file's runs at the head, whose check runs are this round's own jobs or an earlier round's, and are left
# out of both the wait and checks.json.
workflow=${GITHUB_WORKFLOW_REF#"$repo"/}
workflow=${workflow%%@*}
# Without the list, the round's own check runs cannot be told apart, so the wait and checks.json are skipped, as they
# are when the check runs cannot be read.
own=
if runs=$(api_all "repos/$repo/actions/runs?head_sha=$head" workflow_runs); then
	own=$(printf '%s' "$runs" |
		jq -c --arg path "$workflow" '[.[] | select((.path | sub("@.*$"; "")) == $path) | .id]')
else
	echo "::warning::could not list this workflow's runs on $head, so reviewing without waiting for checks and without review/checks.json. The review job needs actions: read."
fi

others() {
	check_runs=$(api_all "repos/$repo/commits/$head/check-runs?filter=latest" check_runs) || return 1
	printf '%s' "$check_runs" | jq -c --argjson own "$own" '[.[]
		| select((.details_url // "") as $u | $own | any(. as $id | $u | contains("/actions/runs/\($id)/")) | not)]'
}

# An automatic round starts before the pull request's checks have reported, while a requested one was asked for by
# someone who judged it ready.
if [ -n "$own" ] && [ "$automatic" = true ] && [ "$LOUPE_CI_WAIT_FOR_CHECKS" -gt 0 ]; then
	start=$(date +%s)
	deadline=$((start + LOUPE_CI_WAIT_FOR_CHECKS * 60))
	while :; do
		if ! checks=$(others); then
			echo "::warning::could not read the check runs on $head; reviewing without waiting"
			break
		fi
		now=$(date +%s)
		pending=$(printf '%s' "$checks" | jq '[.[] | select(.status != "completed")] | length')
		# Slower workflows often have not registered a check run yet, so nothing counts as "all finished" before the
		# grace has passed.
		if [ "$pending" -eq 0 ] && [ "$now" -lt $((start + grace)) ]; then
			sleep "$poll"
			continue
		fi
		if [ "$pending" -eq 0 ]; then
			echo "every other check on $head has finished"
			break
		fi
		if [ "$now" -ge "$deadline" ]; then
			echo "::warning::$pending check run(s) on $head still running after $LOUPE_CI_WAIT_FOR_CHECKS minute(s); reviewing anyway"
			break
		fi
		sleep "$poll"
	done
fi

rm -rf review
mkdir review
git archive "$head" | tar -x -C review
# Written after the archive and with its symlinks gone, so a file in the pull request can neither stand in for the
# files below nor redirect a write out of review/.
find review -type l -exec rm -f {} +
rm -f review/pr.diff review/checks.json
loupe show --run "$run" --diff >review/pr.diff
if [ -z "$own" ]; then
	:
elif checks=$(others); then
	printf '%s' "$checks" | jq '[.[] | select(.status == "completed") | {name, conclusion}]' >review/checks.json
else
	# Absent rather than empty: an empty list would tell the reviewer that nothing ran.
	echo "::warning::could not read the check runs on $head; review/checks.json is not written"
fi

{
	echo "LOUPE_HOME=$LOUPE_HOME"
	echo "LOUPE_RUN=$run"
} >>"$GITHUB_ENV"
{
	echo "skip=false"
	echo "reason="
	echo "run=$run"
	echo "head=$head"
	echo "review_dir=$(pwd)/review"
} >>"$GITHUB_OUTPUT"
