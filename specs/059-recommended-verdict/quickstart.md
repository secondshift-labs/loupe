# Quickstart: check the recommended verdict end to end

Run these from the worktree after `mise run build`. Inside the repository, `loupe` is the dev build.

## Automated

1. `go test ./internal/publish/ -run 'StartAction|ActionRefusal|Schema'`. The `StartAction` table covers each verdict against blocking accepted, blocking excluded, no blocking finding and own pull request (SC-002). The schema test pins schema 8.
2. `go test ./internal/cli/ ./internal/mode/ ./internal/draft/`. These cover every row of the refusal table in `contracts/cli-059.md`, the clear-on-omit rule, and `verdict` in `show --json`.
3. `go test ./internal/tui/`. The action step's goldens cover a start on the verdict, each reason, and no verdict, which matches today's golden byte for byte (SC-004).
4. `go test ./internal/cli/ -update`, then `git diff testdata/golden`. Only `show-help.80.txt` and `show-help.100.txt` change. No `summary` help golden exists, and `show.*.txt` is the text view, which does not print the verdict.
5. `mise run check` after the last edit.

## By eye, through tmux

1. `mise run demo -- review 'acme/widgets#43'`. The demo seeds the verdict `approve`, and the blocking finding is accepted. Press `p`. The cursor is on `request-changes`. The `approve` row opens with `recommended ·`, and the reason row reads `agent recommended approve; f-002 is blocking`.
2. Press esc, open `f-002`, exclude it, and press `p` again. The cursor is on `approve`, and no reason row shows.
3. Press `j`, then enter, then esc. The cursor is back on the action the human chose, not on the start.
4. `mise run demo -- summary --run 'acme/widgets#42' --body x --verdict approve --by human`. Expect `usage` and the fix `drop --verdict; the human picks the action in loupe review`.
5. Repeat step 1 with `NO_COLOR=1` and at 80×24. The mark and the reason read without color, and the reason row stays whole while the cursor's choice still fits.

## Not covered here

No GitHub request changes, so nothing here needs a live pull request.
