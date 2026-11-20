# Research: Start the publish modal on the agent's recommended verdict

Spec 059 left no open marker. Each decision below settles how a requirement maps onto the code as it stands at `3ab7efc`.

## D1. Where the verdict's value is checked

- **Decision**: `internal/cli/summary.go` checks the value against `publish.Actions`. `draft.SetSummary` stores what it is given.
- **Rationale**: `internal/publish` imports `internal/draft`, so the draft cannot read `publish.Actions` without an import cycle. A second list of actions in `draft` would be a copy that can drift. The CLI is also where the two refusal codes differ: `usage` for the flag and `input` for the JSON field (FR-002).
- **Alternatives**: a `draft.Verdicts` list that `publish.Actions` reuses. Rejected, because it moves the publish action list into the draft package for one check.

## D2. Where the start rule lives

- **Decision**: a new `publish.StartAction(verdict, viewer, author string, d *draft.Draft) (action, why string)` in `internal/publish/gates.go`, next to `ActionRefusal`. `why` is empty when the start equals the verdict. A new unexported `blockingIDs(d)` returns the blocking ids of `draft.PublishableSet(d)`, and both `ActionRefusal` and `StartAction` call it (FR-007).
- **Rationale**: the rule needs the blocking set and the availability check, and both are in `gates.go`. A table test in `internal/publish` covers every combination of verdict, blocking decision and own pull request without the review program (SC-002). `ActionRefusal` already writes its messages there, so the reason text has the same home.
- **Alternatives**: the rule in `internal/tui`. Rejected, because it would read the blocking set a second way, which FR-007 forbids, and it could be tested only through the program.

## D3. The reason text when the candidate is refused

- **Decision**: when `ActionRefusal` refuses the candidate with code `own-pr`, `why` ends `you authored this pull request`. A refusal with any other code ends with that refusal's message.
- **Rationale**: the candidate rule already turns `approve` away from a blocking set, so today only `own-pr` can refuse a candidate. The fallback keeps the row truthful if a later spec adds a refusal.

## D4. A verdict that is not one of the actions

- **Decision**: `StartAction` treats a stored verdict outside `publish.Actions` as no verdict.
- **Rationale**: `summary` refuses such a value, so it can only come from a hand-edited draft. A start on an unknown row would leave the cursor nowhere. The schema reader stays as it is, since `docs/versioning.md` governs fields and not values.

## D5. The `recommended` mark

- **Decision**: the verdict's row gets `recommended` and the separator glyph in front of its description, as in `recommended · post the findings and approve the pull request` or `recommended · unavailable: <message>`. It is plain text in the row's secondary tone, like the option picker's mark in `internal/tui/pick.go`.
- **Rationale**: an unavailable row's message can wrap over several lines. A mark at the end would land on the last wrapped line, away from the name. The name column is 16 cells, and `request-changes` fills 15 of them.
- **Alternatives**: a good-tone chip as in the finding's option headings. Rejected, because the popup's rows draw no chips, and the option picker, the nearest precedent, uses plain words.

## D6. The reason row

- **Decision**: `publishStep` takes the reason as a new argument. It wraps the reason to the popup's width in the secondary tone and puts those rows ahead of the notice rows that `withNotice` keeps whole. The rows then sit under the choices and above any refusal. The inline step passes no reason.
- **Rationale**: FR-010 requires a row that stays while the step is open. `m.notice` is cleared on every key in `updateAction`, so the transient notice cannot hold it. `withNotice` already makes the choices give way before a notice is cut, so no id is lost at a short height.
- **Alternatives**: a row above the choices. Rejected, because `choiceWindow` keeps only rows near the cursor when the popup is short, and it would drop the reason first.

## D7. When the start is worked out

- **Decision**: `publishUI.listKey` for `p` (`internal/tui/modeui.go:136`) sets `m.pick` to the start's index and keeps `why` in a new `Model` field, `startWhy`. Esc from the inline step (`list.go:101`) keeps the human's action and leaves `startWhy` as it is.
- **Rationale**: `p` from the list is the only way into the action step with a fresh start (FR-011). The decisions cannot change while the inline step is open, so the reason still holds when the human goes back to the action step.

## D8. Local runs

- **Decision**: `--verdict` on a local run refuses through `mode.apply.Refuses`, which gains a `summary` case for the `verdict` flag: `wrong-mode`, message `loupe summary --verdict applies only to pull request runs`, fix `run loupe summary without --verdict`. A `"verdict"` key on a local run refuses in `runSummary` after the run resolves: `input`, message `input field "verdict" does not apply to local runs`, fix `drop "verdict" from the input`.
- **Rationale**: `resolveRunRoot` already sends every local-run command through `Refuses` with its changed flags, as `show --previous` does. The JSON key needs the decoded input, so it is checked where the input is read, through a new `InputRules.RefuseVerdict`.
- **Amended during implementation**: the plan first checked `target.Kind` in `runSummary`. `TestOnlyTheModeSeamsNameTheLocalKind` keeps run-kind checks inside `internal/mode`, so the key check asks the mode's `InputRules` instead.

## D9. Clearing and `--by human`

- **Decision**: `SetSummary` always assigns the verdict, so an empty one clears it (FR-004). `runSummary` refuses `usage` for a verdict with `--by human` before it resolves the run, fix `drop --verdict; the human picks the action in loupe review`. The same refusal applies to the JSON key.
- **Rationale**: both are usage errors that need no run state, and Principle V asks that `--help` and usage refusals work without one.
