# Unattended reviews in CI

loupe is built for [the attended workflow](review-workflow.md), where a person decides each finding. A CI job can also review a pull request and publish through loupe with no one watching. The review then uses the same [comment format](comment-format.md) as one a person publishes, so human and bot reviews read the same way on the pull request.

## Rules for an unattended review

No person reads an unattended review before it posts, so loupe holds it to fixed rules. These come from Principle II of the [constitution](../.specify/memory/constitution.md).

- It MUST authenticate with a GitHub App installation token. Such a token starts with `ghs_`, and Actions' own `GITHUB_TOKEN` is one. The review posts as that App. A user token is refused with `token`.
- It MUST be a COMMENT review, so it never approves or blocks a merge.
- It is marked unattended in its footer and in its hidden metadata, so it never reads as a person's review.
- It MUST edit only a review that a GitHub App published.
- Its opening comes from the draft's summary. Write the summary for the pull request's author, because no one reads it first.
- It publishes every accepted or pending finding, since no one decides them one by one. A finding that was excluded or withdrawn is left out.

## Install loupe in a job

The repository is also an action. It installs a release, checks the archive against the release's `checksums.txt` and puts `loupe` on `PATH` for the rest of the job. It runs on Linux and macOS, on amd64 and arm64.

```yaml
- uses: secondshift-labs/loupe@<sha> # <tag>
```

Pin the commit SHA of a release tag, with the tag in a comment. The action installs the release it shipped in, so Dependabot moves the action and the binary together. To install another release, set `version`, for example `version: v0.9.0`.

## The pipeline

A pipeline reviews a pull request in four steps: capture, read, file, publish. Only capture needs the Git clone.

1. `loupe capture <pr-url> --json` creates the run and prints its reference as `run`. Pass the reference to later steps in `LOUPE_RUN`, or name it on each step. `show`, `add` and `summary` take `--run <ref>`, and `loupe publish` takes the reference as its argument. Unattended publish never falls back to the current branch's pull request, because a pipeline's checkout is usually not on that branch.
2. `loupe show --diff > review/pr.diff` writes the captured diff for the reviewer to read beside a checkout of the captured head. This is the supported way to get the diff. The run directory's layout is not a contract.
3. File findings with one `loupe add` object or array, which is stored whole or not at all. A finding whose location is not in the captured diff is refused with the nearest valid lines. Then run `loupe summary`, even when there is nothing to report. You write the adapter from your reviewer's output to the input `loupe add` takes.
4. `loupe publish --unattended --json` posts exactly one comment review, with no confirmation. The run records each attempt and its receipt. A pipeline that keeps `LOUPE_HOME` between steps can retry safely, even from another path or machine.

   The job needs `permissions: pull-requests: write`. A pull request of 250 or more commits whose head moved since capture also needs `contents: read`, because GitHub lists at most 250 of a pull request's commits and loupe then compares commits instead. Without `contents: read`, a stacked pull request whose base branch took the captured commit in is refused as head-moved.

## Sticky rounds

A pull request that gets a round on every push collects one review per round. `loupe publish --unattended --sticky` keeps one review current instead. The first sticky round posts a review. Each later round replaces that review's body, with the new round on top and earlier rounds collapsed below it. An edit sends no notification.

Sticky finds the review to edit by the capture's `--source` name. Unattended `--sticky` refuses a run captured without one. Each unattended pipeline on a repository MUST use its own `--source` name, because two that share one pick or mix each other's reviews.

`--note <markdown>` adds a note under the new round's footer, such as how to ask for another round. It shows only while that round is the newest, so pass it again on each round that should carry one. It needs `--unattended --sticky`.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/review-sticky-dark.png">
  <img alt="A sticky review after its third round: the counts and opening for round 3, its two findings as collapsed rows, a footer marked unattended with a link to the changes since round 2, the note under the footer, then an Earlier rounds section with rounds 2 and 1 each collapsed to one line of counts." src="assets/review-sticky.png">
</picture>

## Reusable pieces

A round is two jobs in your workflow file: a review job and a publish job. loupe supplies the parts around your agent. You supply the agent.

- `secondshift-labs/loupe/ci/capture` runs first in the review job. It installs the loupe release that its own commit shipped, captures the pull request and lays out `review/` for the agent. It skips the round when the head is a fork or when the same source already reviewed the head.
- Your agent runs next.
- `secondshift-labs/loupe/ci/save` uploads the run for the publish job.
- `secondshift-labs/loupe/.github/workflows/publish.yml` is a reusable workflow. It restores the run, checks it names this pull request and source, and publishes it unattended. It runs no step of yours.

Spec 058's migration rebuilds the reference Claude reviewer on these pieces as `secondshift-labs/ai-review-workflows`. Until then, loupe reviews its own pull requests through [`.github/workflows/review.yml`](../.github/workflows/review.yml), which calls the older bundled `eriksaulnier/loupe-workflows`.

### Set up a caller

1. Copy the starter file below to `.github/workflows/review.yml`.
2. Pin all three loupe references to the same release commit SHA, with the tag in a comment. Dependabot then moves them together.
3. Set `source` to the same name in `ci/capture`, `ci/save` and `publish.yml`.
4. Replace the Review step with your agent.
5. Create the `ai-review` label. Adding it asks for another round.
6. Review `if:` on the review job. It is your policy.

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

### You bring, loupe gives

| You bring | loupe gives |
| :--- | :--- |
| The agent step, with its model, prompt and credentials | A pinned, checksum-verified loupe on `PATH` |
| The triggers and the `if:` policy on the review job | The captured head and diff in `review/`, and the other checks' results in `review/checks.json` |
| The `source` name | The skip for forks and for a head that `source` already reviewed |
| The `unlabel` job | The upload of the run, and the publish job that restores, checks and posts it |

### Pin `source` and never rename it

`source` MUST be a fixed name of your choice. A sticky round finds its review by that name. A renamed `source` leaves the old review behind and posts a new one. A caller that moves to these pieces MUST set `source` to the name its existing reviews carry, which is the `src=` value in the hidden `loupe-meta` line, before the move.

### Permissions are per job

The workflow sets `permissions: {}` and each job asks for its own. The review job runs an agent on untrusted pull request text, so it MUST NOT hold any `write` permission. Only the publish job holds `pull-requests: write`, and no agent runs in it. A called workflow gets no more than the calling job grants, so the caller MUST grant both permissions on the `publish` call. The `unlabel` job needs `pull-requests: write` for itself.

### The agent's contract

1. Read `review/`. It holds the head's files, `pr.diff` and `checks.json`, the checks on the head that have finished. `checks.json` is absent when `ci/capture` could not read the check runs or this workflow's own runs.
2. Follow `loupe show --help`. It lists the steps in order, including the sticky-round steps.
3. File findings with `loupe add` and finish with `loupe summary`.
4. Do not publish. The publish job does that.

The agent step MUST succeed for the round to publish. `ci/save` runs only after it, so a part-finished draft is never posted. A rerun of the publish job is refused, because an earlier attempt may have posted. Start a fresh round with the label.

## GitHub's behavior

[GitHub facts](github-facts.md) records what GitHub was observed to do with reviews, edits, inline comments, Markdown and Actions tokens. Treat it as evidence, not as a promise about future behavior.
