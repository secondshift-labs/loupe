# Shared CI pieces: capture and publish for any reviewer

*   **Author:** Erik Saulnier
*   **Status:** Approved
*   **Date:** 2026-11-15

--------------------------------------------------------------------------------

## Overview

loupe publishes a review agent's findings as one GitHub review in a fixed format: findings in sections, severity pills, and sticky rounds that edit one review per pull request in place. In GitHub Actions, the only packaged way to get that format today is `eriksaulnier/loupe-workflows`, a reusable workflow that bundles capture, a Claude reviewer and publish into one unit.

This spec splits the bundle. loupe ships three pieces: a `ci/capture` composite action, a `ci/save` composite action, and a reusable workflow, `publish.yml`. A caller runs `ci/capture`, then its own agent, then `ci/save`, in one review job, and calls `publish.yml` as a second job. The agent can be any agent with a shell that can run `loupe`. loupe ships no reviewer, as Principle I requires. `loupe-workflows` moves to `secondshift-labs` as `ai-review-workflows` and becomes the reference Claude reviewer, rebuilt on these pieces. The name says what the repository holds, reusable workflows for AI review, without naming a model or suggesting that loupe ships a reviewer.

This spec covers loupe's side at file level. The work in `loupe-workflows` and in the three repositories that call it is a migration section, because those files live and release elsewhere.

## Background

This section defines the loupe terms the rest of the spec uses.

*   **Run.** One review of one pull request at one head. `loupe capture <pr-url>` creates it. A run is a directory of plain files.
*   **Data root.** The directory that holds every run, set by `LOUPE_HOME`. `LOUPE_RUN` names the run that later commands act on, such as `owner/repo#12`.
*   **The cycle.** `loupe capture` records the pull request's base, head and diff. The reviewer reads the code and the diff, then files findings with `loupe add` and an opening paragraph with `loupe summary`. `loupe publish` posts one review.
*   **Attended and unattended.** On an attended run, a human decides each finding and confirms the post in a terminal. On an unattended run, used in CI, `loupe publish --unattended` posts with a GitHub App installation token, such as Actions' `GITHUB_TOKEN`. The review is a COMMENT, marked unattended (spec 007, Principle II).
*   **Round and sticky.** Each review of a pull request is a round. With `--sticky`, a later round replaces the body of the same review instead of posting a new one, with earlier rounds collapsed below the newest.
*   **Source.** The name of one review series, such as `loupe-ci`. Capture records it, the published review carries it as `src=` in a hidden marker, and a sticky round uses it to find the review to edit. A changed source name finds no review, so the next round posts a second one.
*   **Earlier findings.** On a later round, `loupe show --previous` lists the findings still open from the round before. The reviewer marks each one `open` or `addressed` with `loupe assess`, and files only new problems with `loupe add`.

## The Problem

The goal is to share loupe's review format and organization, not a particular reviewer. The current packaging works against that in three ways.

1.  **The reviewer is bundled with the plumbing.** Someone with their own agent and prompt must fork the whole 950-line workflow (`loupe-workflows/.github/workflows/review.yml`) to replace its Claude prompt, which starts at line 534. A fork stops receiving fixes.
2.  **The fixes in the old workflow are not reusable on their own.** It carries fixes that each took a live run to find: excluding the run's own check runs by run id, skipping a head that was already reviewed, removing the label so that adding it again starts a round, and refusing a rerun that cannot see whether the first attempt posted. A caller who brings their own reviewer gets none of them except by copying shell.
3.  **The rules a reviewer must follow live in a Claude prompt.** The sticky-round steps (assess every earlier finding, never file it again) are prose in the old workflow's prompt (`review.yml:359`) and in `plugin/skills/human-review/SKILL.md`. Principle I says every workflow MUST be completable from `loupe --help`, and this one is not.

## Goals

*   **G1. One file, any agent with a shell.** A repository with an agent that can run `loupe` gets loupe's published format from one workflow file: a review job with `ci/capture`, the agent, and `ci/save`, then a job that calls `publish.yml`. `docs/ci-action.md` carries that file. A live run of it as written, with only the agent step filled in, is an acceptance criterion of this spec.
*   **G2. No agent shares a job with the write token.** `publish.yml` defines the publish job, runs no caller step, and is the only job in the starter file granted `pull-requests: write`. The starter sets `permissions: {}` at workflow level, so no job gets a write token by default. This holds for any caller that keeps the starter's permission lines. The lines remain caller YAML, so it is not enforced against a caller who edits them.
*   **G3. Fixes arrive by pin bump.** Every fix that prevents a wrong or duplicate review lives in loupe's Go code, in `ci/capture` or in `publish.yml`, never in YAML the caller copies. Dependabot's bump of the caller's pins delivers a fix with no other edit.
*   **G4. The reviewer's rules are in `--help`.** An agent with a shell can complete a sticky round from `loupe --help` alone.
*   **G5. A skip that loupe decides says why.** When `ci/capture` skips a round (a fork, or an automatic round at a head already reviewed), it sets `skip` and `reason` outputs and writes the reason to the job summary. A skip that the caller's own `if:` decides is the caller's policy and is silent, as in the old workflow.
*   **G6. The owner's repositories move without a second review.** loupe, market-map and pickle-watch switch to the rebuilt `loupe-workflows`, and each open pull request's review keeps being edited in place.

## Non-Goals

*   **A reviewer shipped or supported by loupe.** Principle I. The reference reviewer lives in `loupe-workflows`.
*   **Trigger policy in loupe's pieces.** Which events start a round, the label name, and whether drafts, bots or release branches skip are each caller's choice, in its own `on:` and `if:`. Label removal is also the caller's, since the label is policy. The starter file ships the owner's current choices as an example.
*   **Fork pull requests under `pull_request_target`.** Spec 007 keeps this out of scope (`specs/007-unattended-publish/spec.md:141`, `:150`). `ci/capture` skips a fork's pull request, as the old workflow does.
*   **Refusing `loupe summary` while earlier findings are unassessed.** It changes attended runs and needs its own rules, so it goes to its own draft spec. `loupe publish` keeps its existing `unassessed` warning (`internal/cli/publish.go:264-293`).
*   **Adapters for non-agent tools.** A linter needs its own step that writes `loupe add` input. FR-024 (`specs/007-unattended-publish/spec.md:187`) says loupe ships no adapter and names no review action in its contract.
*   **CI systems other than GitHub Actions.** The CLI steps in `docs/ci-action.md` stay the path for those.
*   **A hosted GitHub App.**

## Open Questions

*   **Is `loupe-workflows` a second concrete use?** Principle VI asks for a second concrete use before an abstraction. `loupe-workflows` consumes the pieces as a separate repository, and loupe's own CI consumes it. No outside user exists yet. This spec proceeds on the owner's goal of outside adoption and records the departure in Complexity Tracking.
*   **The platform questions are answered.** The probe (Implementation item 1) ran on 2026-11-15, and all three questions passed. `docs/github-facts.md` records the results.

## Critical User Journeys (CUJs)

### CUJ 1: A newcomer gets loupe's format with their own agent

*   **Given**: A GitHub repository with an agent that can run in Actions and run `loupe`, a model credential as a secret, and no loupe setup.
*   **When**: They copy the starter file from `docs/ci-action.md`, set `source`, put their agent's step between `ci/capture` and `ci/save`, and open a pull request.
*   **Then**: One COMMENT review appears in loupe's format, marked unattended, under `github-actions[bot]`. The agent's job held only read permissions.

### CUJ 2: A later round edits the same review

*   **Given**: A pull request with a sticky review from an earlier round, and new commits pushed since.
*   **When**: The caller's trigger starts a round (for the owner's repositories, adding the `ai-review` label).
*   **Then**: The agent follows `loupe show --help`: it reads the earlier findings with `loupe show --previous`, assesses each one, and files only new problems. The existing review's body is replaced with the new round on top. The label is removed when the round ends.

### CUJ 3: An automatic round at an already-reviewed head skips

*   **Given**: A pull request whose current head already has a review from this `source`.
*   **When**: An automatic trigger (`opened`, `ready_for_review` or `synchronize`) fires again.
*   **Then**: `ci/capture` sets `skip` with the reason `already-reviewed`, the agent step and publish do not run, no model tokens are spent, and the job summary says why. A requested round (a label or a dispatch) at the same head runs.

### CUJ 4: The owner's repositories move without a second review

*   **Given**: loupe, market-map and pickle-watch call the old `eriksaulnier/loupe-workflows`, and pull requests are open with loupe reviews.
*   **When**: Each caller pins `source` to the name its reviews already carry, then bumps to the rebuilt `secondshift-labs/ai-review-workflows`.
*   **Then**: The next round on each open pull request finds its existing review by that name. No pull request gets a second loupe review from the same series.

## How

This section is the plan (CONTRIBUTING, "How changes are made"). It holds the architecture, the Constitution Check and Complexity Tracking. The file-level work is in Detailed Implementation Plan.

### Shape

A round is two jobs in the caller's workflow file, plus the caller's own label job.

```text
caller's workflow file: on:, permissions: {}, source
│
├─ job review ─── permissions: contents, pull-requests, checks, actions: read
│                 steps:
│                   secondshift-labs/loupe/ci/capture@<sha>   → outputs skip, reason
│                   <the caller's agent>                      if: skip != 'true'
│                   secondshift-labs/loupe/ci/save@<sha>      if: skip != 'true'
│                 outputs: skip
│
├─ job publish ── uses: secondshift-labs/loupe/.github/workflows/publish.yml@<sha>
│                 needs: review
│                 if: needs.review.result == 'success' && needs.review.outputs.skip != 'true'
│                 permissions: contents: read, pull-requests: write
│
└─ job unlabel ── the caller's own, if: always() and a label started the round
                  permissions: pull-requests: write
```

A round whose review job fails does not publish. `ci/save` runs only when the agent step succeeded, so a part-finished draft is never posted.

`loupe-workflows` becomes one such caller, packaged as a reusable workflow. Its review job runs `claude-code-action` with its prompt, coverage line and trace upload between `ci/capture` and `ci/save`. The owner's three repositories call it.

### `ci/capture` (composite action, new)

It does every reviewer-neutral step before the review, taken from the old workflow's `review` job.

*   **Inputs**: `pull_request` (required), `source` (required), `wait_for_checks` (minutes, default `10`, `0` turns it off), `github-token` (default `${{ github.token }}`).
*   **Automatic or requested.** It reads the trigger from `github.event_name` and `github.event.action`. A `pull_request` event with `opened`, `ready_for_review` or `synchronize` is automatic. Anything else is requested.
*   **Fork skip.** A pull request whose head repository is not `github.repository` sets `skip=true`, `reason=fork`, and stops.
*   **Install.** A `run:` step of its own, before the script. It installs the loupe release its own commit shipped, from its own copy of the repository, so its pin covers the install code. It reads the version from the stamped `default:` line of `$GITHUB_ACTION_PATH/../../action.yml` and runs `$GITHUB_ACTION_PATH/../../install.sh` with `LOUPE_VERSION`, `LOUPE_INSTALL_DIR=$RUNNER_TEMP/loupe-bin` and `LOUPE_NO_PLUGINS=1`. `install.sh` checks the archive against the release's `checksums.txt` (Principle III).
*   **Capture.** It checks out the default branch with full history and `persist-credentials: false`, so no token is written to the clone's git config. It runs `loupe capture <url> --source <source> --json` with `LOUPE_HOME=$RUNNER_TEMP/loupe-home`. The token reaches git for that one command only, through `GIT_CONFIG_COUNT=1`, `GIT_CONFIG_KEY_0=http.https://github.com/.extraheader` and `GIT_CONFIG_VALUE_0=AUTHORIZATION: basic <base64 of x-access-token:token>` in that command's environment. loupe passes `GIT_CONFIG_*` through to git (`internal/gitenv/gitenv.go:11-14` filters only repository-selecting variables). It reads the result from stdout. A refusal fails the step and prints loupe's `error:` and `fix:` lines. A refusal is not a skip.
*   **Already reviewed.** On an automatic round, when `previous.head` from the capture result equals the pull request's head, it sets `skip=true`, `reason=already-reviewed`. A requested round always runs. When `previous.head` is absent, the round runs.
*   **Wait for checks.** On an automatic round with `wait_for_checks` above 0, it polls the head's check runs until each finishes or the time runs out. It leaves out check runs that belong to this workflow file's own runs at the head, matched by run id in each check run's `details_url`. It writes the finished checks to `review/checks.json`.
*   **Review directory.** It writes the head's files to `review/` with `git archive` and the captured diff to `review/pr.diff` with `loupe show --diff`. The agent reads `review/`.
*   **Credentials.** No token is stored in `.git` and none is exported to later steps, so the agent's step can read no token from the clone or the environment.
*   **Environment.** It exports `LOUPE_HOME` and `LOUPE_RUN` for the steps after it, and puts `loupe` on `PATH`.
*   **Outputs**: `skip` (`true` or `false`), `reason` (`fork`, `already-reviewed` or empty), `run`, `head`, `review_dir`. A skip also writes its reason to `$GITHUB_STEP_SUMMARY`.
*   **Scripts.** Everything after the install lives in `ci/capture/capture.sh`, which the action runs from `$GITHUB_ACTION_PATH`. It reads JSON with `jq`, which GitHub's runners carry, and calls the REST API with `curl` against `$GITHUB_API_URL`, so tests can point it at the fake GitHub and put a stub `loupe` on `PATH`. The wait polls every `LOUPE_CI_POLL_SECONDS` seconds (default 15) after a grace of `LOUPE_CI_GRACE_SECONDS` (default 60), as the old workflow does, and tests set both to 0.

### `ci/save` (composite action, new)

It uploads the data root as artifact `loupe-<pr>-<source>-reviewed`, with `include-hidden-files: true`. Without that flag the upload silently drops loupe's files and publish finds no findings. It needs no loupe install. Inputs: `pull_request`, `source`.

### `publish.yml` (reusable workflow, new)

It is the old workflow's `publish` job, with these changes: it installs loupe from its own commit, it checks the run it restores, and it has a job-level concurrency group. Its unlabel job moves to the caller.

*   **Inputs**: `pull_request` (required), `source` (required), `sticky` (boolean, default `false`, as in the old workflow), `note` (default empty).
*   **Permissions**: `contents: read`, `pull-requests: write`. It runs no caller step and checks out nothing from the pull request (FR-030).
*   **Concurrency**: the job-level group `loupe-publish-<pr>-<source>`, `cancel-in-progress: false`.
*   **Install.** It reads its own commit from `job.workflow_sha`, through `toJSON(job)` because actionlint 1.7.12 does not know the field (`github.workflow_sha` is the caller's commit, item 1a), checks out only `action.yml` from `secondshift-labs/loupe` at that commit into `.loupe/`, and runs it as the local action `./.loupe`. That installs the release the caller pinned, with no version string in `publish.yml`.
*   **Rerun guard.** When `github.run_attempt` is above 1, the job refuses, because an earlier attempt may have posted and nothing this job can trust records whether it did. An artifact could, but the review job, which runs the agent, can upload one under any name. The way back is a fresh round. On attempt 1 it downloads the `-reviewed` artifact.
*   **Trust check.** The `-reviewed` artifact was written by a job that ran an agent on untrusted pull request text. Before publishing, it reads the run with `loupe show --run <ref> --json` and refuses unless `target.owner/target.repo` is `github.repository`, `target.number` is `pull_request` and `target.source` is `source`. Everything else in the run, such as the findings and the summary, is the reviewer's output and is published as such. loupe's Markdown allowlist (`docs/comment-format.md`) applies to it.
*   **Publish.** `loupe publish <ref> --unattended --json`, plus `--sticky` and `--note` when set. A result with `replayed: true` fails the job, since this job restores no receipt of its own and a replay means the restored run carried one. `review_url` is written only when it matches `https://github.com/<repo>/pull/<n>#pullrequestreview-<id>`. It uploads the data root as `loupe-<pr>-<source>-published` with `if: always()`, as evidence that nothing reads back.
*   **Outputs**: `review_url`.

### Change to the loupe binary

`loupe capture --json` adds `head` to its `previous` result: the commit the previous round reviewed. For `from: "github"` it is the round's commit, which capture already reads back (`Previous.Commit`, `internal/publish/previous.go:28`). For a sticky review that is the newest round's commit from its body. For `from: "receipt"` it is that round's `target.headSha`. It is absent when `previous.from` is `none` or the commit could not be read back. The change only adds a field. No run file changes, so no schema bumps.

`loupe show --help` gains the sticky-round steps in the order an agent runs them: `show --previous`, `assess` every `earlier` entry as `open` or `addressed`, `add` only new problems, then `summary`. That makes the round completable from `--help` alone (Principle I, G4).

### Versioning

`ci/capture` and `publish.yml` each install the release that matches their own commit, as described above. `ci/save` installs nothing. A caller pins all three by the same release SHA, so Dependabot moves them together. Only the root `action.yml` carries a stamped version, as today, and release-please stamps nothing new.

### Moving existing callers (G6)

A caller MUST set `source` to the name its existing reviews carry before it moves, because a sticky round finds its review by that name. The old workflow derived a default name from the caller's workflow file and job, and the new pieces derive nothing. loupe's caller already pins `loupe-ci`. pickle-watch sets neither `source` nor `sticky` (`pickle-watch/.github/workflows/review.yml:31`), so its reviews are not sticky, and the next round posts a new review as it does today. market-map's caller was not checked.

### What stays out of loupe

The prompt, the model and provider inputs, `max_turns`, the instructions file, the coverage line and the trace upload stay in `loupe-workflows`. They are the reviewer.

### Constitution Check

| Principle | Result | How the design meets it |
| :--- | :--- | :--- |
| I. A tool for agents, not a tool that uses agents | Pass | loupe ships no reviewer. `ci/capture`, `ci/save` and `publish.yml` run loupe, git, curl and GitHub's own actions. `ci/capture` prepares files for an agent and starts none. The sticky-round steps move into `loupe show --help`. |
| II. Nothing posts unread under a human's name | Pass | The unattended path does not change: installation token, COMMENT review, marked unattended, edits only an App's review. |
| III. Local files, no service | Pass | No server or database. Every install checks the release against its `checksums.txt`. |
| IV. Never touch the user's checkout | Pass | The binary's capture behavior does not change. The pieces check out and archive inside the CI runner's own workspace, which is not a user's clone. |
| V. Machine contract first | Pass | The capture result only gains a field. No new refusal code and no run file change. |
| VI. Simplicity over ceremony | Departure | Three new pieces with no outside consumer yet. See Complexity Tracking. No new runtime dependency. |
| VII. Verified means ran | Departure | The pieces' wiring is verified by a live run. See Complexity Tracking. The binary change and `capture.sh` are tested against the fake GitHub. |

### Complexity Tracking

| Departure | Why it is needed | Simpler alternative rejected because |
| :--- | :--- | :--- |
| Principle VI: three pieces with one consumer so far, `loupe-workflows` | The goal is adoption outside the owner's repositories, and a second consumer cannot appear until the pieces exist | Moving `loupe-workflows` and changing nothing else leaves the format bundled with one reviewer |
| Principle VII: the pieces' wiring is verified by a live run on `loupe-probe#1`, the owner's private test repository | GitHub Actions cannot run under `go test` | `capture.sh` runs under `go test` against the fake GitHub, parse tests pin the structure, and the live run goes on the Unverified list until the owner runs it |
| Spec 007 FR-030 | FR-030 says every job that runs loupe builds it from `main`, and lists the reviewing job's permissions exactly. These pieces install a checked release instead, and the review job also needs `actions: read` to tell its own check runs apart | This spec amends FR-030 (Implementation item 17) instead of overriding it |

## Risks

*   **A caller grants `pull-requests: write` to the review job.** The agent then holds a write token. Mitigation: the starter sets `permissions: {}` at workflow level and grants per job, and `docs/ci-action.md` says why. Not enforceable from loupe.
*   **The agent reads the read-only token.** The review job's `GITHUB_TOKEN` can read the repository and its pull requests. `ci/capture` never stores or exports it, but an agent that the caller passes `${{ github.token }}` to can still read it. It cannot write. On a private repository, an agent can already read the code, so the token adds little.
*   **A prompt-injected agent writes a misleading run.** The trust check stops a post to another pull request or series. It cannot stop false findings, which is the same risk as any unattended review.
*   **Two rounds at the same head.** Two requests in quick succession both run, spend two model runs, and edit the same sticky review twice. The starter's concurrency group on the review job queues them. It sits on the job, not the workflow, because GitHub keeps one waiting run per group, and a run whose review job is skipped would otherwise cancel a waiting round. Without it, the publish job's group still serializes the posts, and `loupe publish` refuses a stale round with `head-moved` when the head has moved.
*   **A rerun cannot publish.** "Re-run failed jobs" makes attempt 2, and publish refuses every attempt after the first. The fix is a fresh round by the label, as today. Safe reruns are a Future Improvement.
*   **Size.** The artifact is the data root: the diff and the run's JSON files. It grows with the diff, not the repository. A diff past the artifact size limit fails the upload, and the step fails. No limit beyond GitHub's own applies.

## Alternatives Considered

### Alternative A: Merge `loupe-workflows` into loupe as it is

*   **Pros**: One repository and one release.
*   **Cons**: loupe would ship a reviewer and prompt it. Workflow fixes would wait for a loupe release.
*   **Reason for Rejection**: Rejected by author: it breaks Principle I (`.specify/memory/constitution.md:9`).

### Alternative B: Amend Principle I so CI integrations in the repository MAY prompt a reviewer

*   **Pros**: Everything lives in one repository.
*   **Cons**: It weakens the rule that keeps loupe reviewer-neutral. README's "brings no review method" stops being true.
*   **Reason for Rejection**: Rejected by author: keeping the reviewer in `loupe-workflows` meets the goal with no amendment.

### Alternative C: A hosted GitHub App

*   **Pros**: One-click install, no workflow YAML, a branded bot identity.
*   **Cons**: Needs a server and someone holding the App's private key. Either loupe pays for every caller's model calls or callers hand over their keys.
*   **Reason for Rejection**: Rejected by author: no revenue is intended, and it works against bringing your own reviewer.

### Alternative D: Move `loupe-workflows` and change nothing else

*   **Pros**: Smallest change.
*   **Cons**: Bringing another agent still means forking the whole workflow.
*   **Reason for Rejection**: Rejected by author: it misses the goal.

### Alternative E: Three jobs, with `capture.yml` as a reusable workflow before the review job

*   **Pros**: The skip happens before the agent's job starts. The agent's job holds no GitHub read permission.
*   **Cons**: Capture's fetched code stays in the capture job's clone, so the review job needs the head tree shipped in an artifact or a fresh checkout. It adds a `restore` action and a second artifact transfer. Waiting for checks costs the same runner time in either job.
*   **Reason for Rejection**: Rejected by author after review: more pieces for no saving, with only one consumer.

### Alternative F: Composite actions for publish too, with no reusable workflow

*   **Pros**: Every piece can carry a stamped version.
*   **Cons**: The caller writes the publish job, so keeping an agent out of the write-token job rests only on the docs.
*   **Reason for Rejection**: Rejected by author: `publish.yml` keeps that rule in loupe's file.

### Alternative G: Parse the review's hidden marker with jq to detect an already-reviewed head

*   **Pros**: No CLI change.
*   **Cons**: A second parser of `docs/comment-format.md` lives outside Go. A marker change would break the skip silently.
*   **Reason for Rejection**: Rejected by author: capture already reads the previous review back, so it reports the head as one field.

### Alternative H: Also report when the previous review was last edited

*   **Pros**: A requested round could skip when another round posted since it was queued, as the old workflow does.
*   **Cons**: A new GraphQL lookup, a node id on reviews, fake GitHub support and a run file schema bump, to save one model run in a rare double request.
*   **Reason for Rejection**: Rejected by author after review: the starter's concurrency group and the sticky edit contain the cost.

### Alternative I: Derive `source`, or default it to a fixed name

*   **Pros**: No input for the caller to set.
*   **Cons**: Derivation is the most error-prone part of the old workflow, and its result changes under the new layout. A fixed default mixes two series on one repository silently.
*   **Reason for Rejection**: Rejected by author: a required input costs one line.

### Alternative J: Label removal inside `publish.yml`

*   **Pros**: One less job in the caller.
*   **Cons**: It must run when publish does not, such as after a failed or skipped review, which needs extra inputs and an `always()` call.
*   **Reason for Rejection**: Rejected by author after review: the label is caller policy.

### Alternative K: The caller uses `actions/upload-artifact` directly instead of `ci/save`

*   **Pros**: One less piece.
*   **Cons**: The artifact name and `include-hidden-files: true` are copied into each caller, and a mistake in either fails silently.
*   **Reason for Rejection**: Rejected by author: those lines belong in pinned code (G3).

## Future Improvements

*   **Refuse `loupe summary` while earlier findings are unassessed**, in its own spec. It needs rules for stale assessments, for previous rounds `assess` refuses, and for where in `summary` the check runs.
*   **Safe reruns.** Derive the publication id from run, head and digest, so attempt 2 can see attempt 1's review. Needs an amendment to the marker rules in `docs/comment-format.md`.
*   **Bring your own App identity.** A section in `docs/ci-action.md` on minting an App token with `actions/create-github-app-token` in the publish job.
*   **A second reference reviewer**, such as a small gadfly-style review method, once it exists.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

Items run in this order. Item 1 comes first because each of its three answers can switch an item to its fallback.

### 1. Probe on `loupe-probe#1` (owner-run, before any other item)

*   **Type / Scope**: Live check. It posts to a live pull request, so it runs only when the owner asks for it by name. Throwaway workflows on a branch of `loupe-probe`, removed afterward.
*   **1a. Can a called workflow read its own commit?** A caller calls a reusable workflow in another repository by SHA. The called job prints every `github` and `job` context field that names a workflow ref or SHA, such as `github.workflow_sha`, `github.job_workflow_sha` and `job.workflow_sha`. **Pass**: one field equals the SHA the caller pinned. **Fallback on fail**: item 11 installs with `uses: secondshift-labs/loupe@v<version> # x-release-please-version`. `release-please-config.json` adds `.github/workflows/publish.yml` as a `generic` extra-file. `.github/workflows/release.yml` passes `token: ${{ secrets.RELEASE_PLEASE_TOKEN }}` to release-please, a fine-grained token with contents, pull-requests and workflows write that the owner creates. `TestInstallAction` (`internal/cli/action_test.go:10`) also requires the marker in `publish.yml`.
*   **1b. Does an action in a subdirectory get the whole repository?** A job uses an action from a subdirectory of another repository at a SHA and lists `$GITHUB_ACTION_PATH/../..`. **Pass**: `action.yml` and `install.sh` are there. **Fallback on fail**: `ci/capture`'s install step becomes `uses: secondshift-labs/loupe@v<version> # x-release-please-version`, a tag that can move after release. `release-please-config.json` adds `ci/capture/action.yml` as a `generic` extra-file. Item 12's `TestCapturePieceCarriesNoVersion` becomes a stamp test for that file, in the style of `TestInstallAction`.
*   **1c. Does the write permission reach two levels below the caller?** A caller calls reusable workflow A, granting `pull-requests: write` on that call. A calls reusable workflow B, granting `pull-requests: write` on its call. A job in B adds and removes a label. **Pass**: the label call succeeds. **Fallback on fail**: no loupe file changes. Migration step 3 changes instead: `loupe-workflows` ships its Claude review job as a composite action, and its callers call loupe's `publish.yml` directly as their second job.
*   **Result**: run 2026-11-15, all three passed, so no fallback applies. `docs/github-facts.md` (item 16) records the results.

### Binary

#### 2. `internal/cli/capture.go` (Modify)

*   **Type / Scope**: CLI command.
*   **Proposed Changes**: In `capturePrevious` (line 411), add `"head": p.Commit` to the `from: "github"` map when `p.Commit` is not empty. For `from: "receipt"`, load the previous round's target with `run.LoadTarget(run.RunDir(...))`, as `capture.go:436-437` does, and add `"head": <its HeadSHA>`. When that load fails, leave `head` out. Capture does not refuse. Update the `--help` example (line 77) and the text at line 81 to list `head`.

#### 3. `internal/cli/capture_test.go` (Modify)

*   **Type / Scope**: Unit test.
*   **Proposed Changes**: `previous.head` for a github previous round, for a sticky review with a readable round commit, its absence for a sticky body whose commit cannot be read back, its value for a receipt previous round, its absence when the receipt round's `target.json` cannot be loaded, and its absence for `from: "none"`.

#### 4. `internal/cli/show.go` (Modify)

*   **Type / Scope**: CLI help.
*   **Proposed Changes**: `show --help` gains this help text: "Later rounds: run `loupe show --previous`. Assess every `earlier` entry with `loupe assess <refs> --status open|addressed`. File only new problems with `loupe add`, then run `loupe summary`. An earlier finding left unassessed does not carry forward to the next round."

#### 5. `internal/cli/golden_test.go` and `testdata/golden/cli/` (Modify, and create two goldens)

*   **Type / Scope**: Goldens.
*   **Proposed Changes**: Add `{"capture-help", []string{"capture", "--help"}}` and `{"show-help", []string{"show", "--help"}}` to `TestHumanOutputGoldens` (`golden_test.go:77-87`). Create their golden files with `go test ./internal/cli/ -update` and read them.

#### 6. `internal/integration/sticky_test.go` (Modify)

*   **Type / Scope**: End-to-end test against the local remote and the fake GitHub.
*   **Proposed Changes**: Two sticky rounds by an installation token. Round 2 captured into the same data root reports `previous.from: "receipt"` with `previous.head` equal to round 1's captured head. Round 2 captured into a fresh data root, as CI does, reports `previous.from: "github"` with the same `previous.head`.

### CI pieces

#### 7. `internal/testutil/fakegh/fakegh.go` and `fakegh_test.go` (Modify)

*   **Type / Scope**: Test helper. Item 9 depends on it.
*   **Proposed Changes**: Add `SeedCheckRun(owner, repo, sha string, r CheckRun)` with `Name`, `Status`, `Conclusion` and `DetailsURL`, served at `GET /repos/{o}/{r}/commits/{sha}/check-runs` as `{"total_count": n, "check_runs": [...]}`. Add `SeedWorkflowRun(owner, repo string, r WorkflowRun)` with `ID`, `Path` and `HeadSHA`, served at `GET /repos/{o}/{r}/actions/runs?head_sha=` as `{"total_count": n, "workflow_runs": [...]}`. `wirePR` (`fakegh.go:889-893`) adds `head.repo.full_name` when `HeadOwner` is seeded.

#### 8. `ci/capture/action.yml` and `ci/capture/capture.sh` (Create)

*   **Type / Scope**: Composite action and its script.
*   **Proposed Changes**: As in the How section. `action.yml` holds the inputs and outputs and three steps: `actions/checkout` pinned by SHA (default branch, `fetch-depth: 0`, `persist-credentials: false`), the install `run:` step, and one `run:` step that calls `"$GITHUB_ACTION_PATH/capture.sh"` with the inputs as environment variables. `capture.sh` (POSIX sh, `set -eu`) runs, in order: fork check, capture, already-reviewed check, wait for checks, review directory, environment export, outputs. The fork check reads `.head.repo.full_name` from `GET /repos/{owner}/{repo}/pulls/{n}`, and a null `head.repo`, a deleted fork, counts as a fork. All REST calls use `curl --fail-with-body` with GET and `Authorization: Bearer $GITHUB_TOKEN` against `$GITHUB_API_URL`. Own runs are the workflow runs of the caller's workflow file at the head: `GET /repos/{owner}/{repo}/actions/runs?head_sha=<head>`, kept where `path` equals the workflow file named in `GITHUB_WORKFLOW_REF`. Every check run whose `details_url` contains `/actions/runs/<id>/` for one of them is left out.

#### 9. `internal/cli/capture_script_test.go` (Create)

*   **Type / Scope**: Script test, in the style of `TestInstallScript*` in `install_script_test.go`, with a stub on `PATH` as `gitfail` does (`internal/testutil/gitfail/gitfail.go:40-52`).
*   **Proposed Changes**: Runs `ci/capture/capture.sh` directly, without the install step, with `GITHUB_API_URL` set to a `fakegh` server, `LOUPE_CI_POLL_SECONDS=0`, `LOUPE_CI_GRACE_SECONDS=0`, a `gitrepo` clone as the working directory, and a stub `loupe` first on `PATH` that prints a chosen capture result and records its environment. Cases: fork skip (seeded `HeadOwner` differs), deleted-fork skip (no `HeadOwner`), already-reviewed skip on an automatic round, a requested round at the same head runs, `previous.head` absent runs, own-run check runs left out of the wait, a capture refusal fails the step with loupe's `error:` and `fix:` lines, the stub saw `GIT_CONFIG_COUNT=1`, and neither `.git/config` nor `$GITHUB_ENV` holds the token afterward.

#### 10. `ci/save/action.yml` (Create)

*   **Type / Scope**: Composite action.
*   **Proposed Changes**: Inputs `pull_request` and `source`. One `actions/upload-artifact` step pinned by SHA: name `loupe-${pull_request}-${source}-reviewed`, path `$LOUPE_HOME`, `include-hidden-files: true`, `if-no-files-found: error`.

#### 11. `.github/workflows/publish.yml` (Create)

*   **Type / Scope**: Reusable workflow (`on: workflow_call` only).
*   **Proposed Changes**: As in the How section. One job, `publish`, with the permissions and concurrency group above. Steps, each action pinned by SHA: read own commit (item 1a's answer); `actions/checkout` of `secondshift-labs/loupe` at that commit with `sparse-checkout: action.yml` into `.loupe`; `uses: ./.loupe`; refuse when `github.run_attempt` is above 1; download `-reviewed`. Then the trust check with `loupe show --json`, `loupe publish --unattended --json` with `--sticky` and `--note` when set, the `replayed` and `review_url` checks, and the upload of `-published` with `if: always()` and `include-hidden-files: true`. Steps use `gh`, `jq` and `$GITHUB_OUTPUT`. The output `review_url` comes from the publish result. Carries over the old job's live-run fixes: `pull_request` is a string input, and a refusal is read from stdout.

#### 12. `internal/cli/action_test.go` (Modify)

*   **Type / Scope**: Unit test.
*   **Proposed Changes**: Three new tests, parsing YAML with `gopkg.in/yaml.v3`:
    1.  `TestPublishWorkflowRunsNoCallerCode`: every `uses:` in `publish.yml` is on an allowlist (`actions/checkout`, `actions/upload-artifact`, `actions/download-artifact`, `./.loupe`). The `actions/checkout` step MUST name `repository: secondshift-labs/loupe` and `sparse-checkout: action.yml`. The workflow declares `on: workflow_call` only.
    2.  `TestCIPiecesPinBySHA`: every `uses:` in `ci/capture/action.yml`, `ci/save/action.yml` and `publish.yml` is a 40-character SHA with a version comment, except `./.loupe`.
    3.  `TestCapturePieceCarriesNoVersion`: `ci/capture/action.yml` and `capture.sh` contain no `x-release-please-version` marker and no `secondshift-labs/loupe@` reference, so the install follows the pin.

#### 13. `go.mod`, `mise.toml`, `.github/dependabot.yml` (Modify)

*   **Type / Scope**: Build.
*   **Proposed Changes**: `go.mod` moves `gopkg.in/yaml.v3` from indirect to direct, for the parse tests only. It is already in the module graph (`go.sum:135`). `mise.toml`'s `check` task adds `shellcheck ci/capture/capture.sh` beside `shellcheck install.sh`. `dependabot.yml`'s `github-actions` entry (line 4) changes `directory: /` to `directories: ["/", "/ci/capture", "/ci/save"]`, so the pins inside the two actions get bumps. Whether Dependabot reads composite actions in subdirectories is unverified. Check it on the first Dependabot run after merge.

### Docs and contracts

#### 14. `specs/001-loupe-v1/contracts/cli.md` (Modify)

*   **Type / Scope**: CLI contract.
*   **Proposed Changes**: The `capture` entry (line 104) gains `previous.head`, citing `specs/058-ci-workflows`.

#### 15. `docs/ci-action.md` (Modify)

*   **Type / Scope**: Docs.
*   **Proposed Changes**: Replace "The shared review workflow" (from line 50 up to "GitHub's behavior" at line 59) with "Reusable pieces". It holds the starter file below, a numbered setup list, a "you bring, loupe gives" table, the `source` warning (pin it before moving, never rename it, because a sticky round finds its review by that name), why permissions are granted per job, and the agent's contract: read `review/`, follow `loupe show --help`. It links `secondshift-labs/ai-review-workflows` as the reference Claude reviewer. The starter file, with `<sha>` and `vX.Y.Z` filled in at release:

```yaml
name: review

on:
  workflow_dispatch:
    inputs:
      pull_request:
        description: Pull request number to review
        required: true
        type: string
  pull_request:
    types: [opened, ready_for_review, labeled]

permissions: {}

jobs:
  review:
    # Policy: edit freely. A label or a dispatch overrides the automatic skips.
    if: >-
      github.event_name == 'workflow_dispatch' ||
      (github.event.action == 'labeled' && github.event.label.name == 'ai-review') ||
      (github.event.action != 'labeled' && !github.event.pull_request.draft &&
       !startsWith(github.head_ref, 'release-please--') &&
       !endsWith(github.event.pull_request.user.login, '[bot]'))
    runs-on: ubuntu-latest
    # Up to 10 minutes of waiting for checks, plus the agent's own time.
    timeout-minutes: 30
    # On the job, not the workflow: only runs that pass the if above take a place, so an unrelated label cannot
    # cancel a round that is waiting its turn.
    concurrency:
      group: loupe-review-${{ inputs.pull_request || github.event.pull_request.number }}
      cancel-in-progress: false
    permissions:
      contents: read
      pull-requests: read
      checks: read
      actions: read
    outputs:
      skip: ${{ steps.capture.outputs.skip }}
    steps:
      - id: capture
        uses: secondshift-labs/loupe/ci/capture@<sha> # vX.Y.Z
        with:
          pull_request: ${{ inputs.pull_request || github.event.pull_request.number }}
          source: my-reviewer
      - name: Review
        if: steps.capture.outputs.skip != 'true'
        # Your agent goes here. It reads review/ and follows `loupe show --help`.
        run: exit 1
      - if: steps.capture.outputs.skip != 'true'
        uses: secondshift-labs/loupe/ci/save@<sha> # vX.Y.Z
        with:
          pull_request: ${{ inputs.pull_request || github.event.pull_request.number }}
          source: my-reviewer

  publish:
    needs: review
    if: needs.review.result == 'success' && needs.review.outputs.skip != 'true'
    permissions:
      contents: read
      pull-requests: write
    uses: secondshift-labs/loupe/.github/workflows/publish.yml@<sha> # vX.Y.Z
    with:
      pull_request: ${{ inputs.pull_request || github.event.pull_request.number }}
      source: my-reviewer
      sticky: true

  unlabel:
    needs: [review, publish]
    if: >-
      always() && github.event.action == 'labeled' && github.event.label.name == 'ai-review' &&
      github.event.pull_request.head.repo.full_name == github.repository
    runs-on: ubuntu-latest
    timeout-minutes: 5
    permissions:
      pull-requests: write
    steps:
      - env:
          GH_TOKEN: ${{ github.token }}
          LABEL_URL: repos/${{ github.repository }}/issues/${{ github.event.pull_request.number }}/labels/ai-review
        run: |
          if ! gh api --method DELETE "$LABEL_URL" 2> err.txt; then
            grep -q 'HTTP 404' err.txt || { cat err.txt >&2; exit 1; }
          fi
```

#### 16. `docs/github-facts.md` (Modify)

*   **Type / Scope**: Docs.
*   **Proposed Changes**: Record item 1's three answers with the date.

#### 17. `specs/007-unattended-publish/spec.md` (Modify)

*   **Type / Scope**: Spec amendment.
*   **Proposed Changes**: FR-030 (line 196) becomes: "Every job that runs loupe MUST install a loupe release checked against its `checksums.txt`, never build it from the pull request's head. The reviewing job MUST NOT declare any `write` permission. Among jobs that run loupe or an agent, only the publishing job MAY declare `pull-requests: write`, and an agent MUST NOT run in it." Add "(amended on <date> by `specs/058-ci-workflows`)".

#### 18. `AGENTS.md` (Modify)

*   **Type / Scope**: Agent docs.
*   **Proposed Changes**: Add `ci/` to the layout table. Add `publish.yml` to the `.github/workflows/` row. Update the `review.yml` row with the new `loupe-workflows` location. The `install.sh` row (line 34) adds that `ci/capture` runs it, and the `action.yml` row (line 35) adds that `publish.yml` runs it as a local action.

#### 19. `specs/001-loupe-v1/validation.md` (Modify)

*   **Type / Scope**: Validation record.
*   **Proposed Changes**: Add to the Unverified list (line 91): item 20's acceptance run and migration step 4, until the owner runs them.

### 20. Acceptance run of the starter file (owner-run, after the release)

*   **Type / Scope**: Live check on `loupe-probe#1`, run only when the owner asks for it by name. It is G1's acceptance.
*   **Proposed Changes**: The starter file from item 15, as written, with the Review step replaced by `loupe summary --by agent --expect-findings 0 --body 'Acceptance run.' --json`. **Pass**: one COMMENT review posts, marked unattended. Adding the label again starts a second round, which edits the same review. The label is gone after each round.

### Migration (outside this repository)

Done after the loupe release that ships items 2-19, in this order.

1.  **Pin `source` in every caller.** market-map and pickle-watch set `source:` to the name in their reviews' hidden `loupe-meta` line (`src=`). loupe's caller pins `loupe-ci` (`.github/workflows/review.yml:40-43`).
2.  **Transfer `eriksaulnier/loupe-workflows` to `secondshift-labs` and rename it `ai-review-workflows`.** GitHub redirects the old name. In that repository, update its README's links.
3.  **Rebuild `loupe-workflows` as v3.0.0.** Its `review.yml` becomes a review job (`ci/capture`, the Claude step with its prompt, model, provider, `max_turns`, instructions file, coverage line and trace upload, `ci/save`), a call to loupe's `publish.yml`, and an `unlabel` job. `source` becomes required. Its draft, bot and release-branch skips move into its review job's `if:`, so its callers keep today's behavior. Its `sticky` default stays `false`. If item 1c failed, use that item's fallback shape instead.
4.  **Try it on `loupe-probe#1`**: two sticky rounds, then a rollback to the old pin. **Pass**: round 2 edits round 1's review, and the old pin's next round still edits it.
5.  **Move each caller's pin** to `secondshift-labs/ai-review-workflows@<sha> # v3.0.0`. loupe moves last, since its own reviews check every later loupe change.
6.  **Add one-click setup for the organization's repositories.** Create `secondshift-labs/.github` with `workflow-templates/ai-review.yml`, the short caller of `ai-review-workflows` at its v3.0.0 release SHA with `source` and the model left for the repository owner to fill in, and its `ai-review.properties.json`. A template is copied when a repository adds it, so later releases reach those copies through Dependabot, not the template.

### Verification Plan

*   **Compilation/Build**: `mise run build`.
*   **Tests and lint**: `mise run check`, which runs `go vet`, `golangci-lint`, `actionlint` over `.github/workflows/` (it covers `publish.yml`), `shellcheck` over `install.sh` and `capture.sh`, `scripts/check-tests.sh` and `go test ./...`. The parse tests in item 12 cover the two composite actions, which `actionlint` does not read.
*   **Coverage**: no drop in the Codecov total. Every new branch in `capture.go` is covered.
*   **Live**: item 1 before the work, item 20 and migration step 4 after. All three are owner-run and stay on the Unverified list until they run.
