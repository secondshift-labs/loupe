package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/secondshift-labs/loupe/internal/gitenv"
	"github.com/secondshift-labs/loupe/internal/github"
	"github.com/secondshift-labs/loupe/internal/testutil/fakegh"
	"github.com/secondshift-labs/loupe/internal/testutil/gitrepo"
)

// Stands in for the installed loupe: capture records its environment and prints the result the test chose, and show
// prints a diff.
const stubLoupe = `#!/bin/sh
printf '%s\n' "$*" >> "$STUB_DIR/calls"
case "$1" in
capture)
	env > "$STUB_DIR/capture.env"
	cat "$STUB_DIR/capture.json"
	exit "$(cat "$STUB_DIR/capture.exit")"
	;;
show)
	printf 'diff --git a/src/app.go b/src/app.go\n'
	;;
esac
`

const captureToken = "ghs_stand-in-token-value"

type captureFixture struct {
	gh      *fakegh.Server
	repo    *gitrepo.Repo
	stub    string
	tmp     string
	output  string
	env     string
	summary string
	vars    map[string]string
}

func newCaptureFixture(t *testing.T) *captureFixture {
	t.Helper()
	root := t.TempDir()
	f := &captureFixture{
		gh:      fakegh.New(t),
		repo:    gitrepo.New(t, "o", "r", 3),
		stub:    filepath.Join(root, "stub"),
		tmp:     filepath.Join(root, "runner-temp"),
		output:  filepath.Join(root, "output"),
		env:     filepath.Join(root, "env"),
		summary: filepath.Join(root, "summary"),
	}
	bin := filepath.Join(root, "bin")
	for _, dir := range []string{f.stub, f.tmp, bin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExecutable(t, filepath.Join(bin, "loupe"), stubLoupe)
	// What loupe capture leaves in the clone: the head's objects, which git archive reads.
	f.repo.Git("fetch", "--quiet", "origin", "refs/pull/3/head")
	f.gh.SetPR("o", "r", github.PullRequest{Number: 3, State: "open", HeadSHA: f.repo.HeadSHA(), HeadOwner: "o", HeadRepo: "r"})
	f.captureResult(t, map[string]any{"from": "none", "reason": "no earlier loupe review"}, 0)
	f.vars = map[string]string{
		"PATH":                     bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"STUB_DIR":                 f.stub,
		"GITHUB_API_URL":           f.gh.URL,
		"GITHUB_REPOSITORY":        "o/r",
		"GITHUB_TOKEN":             captureToken,
		"GITHUB_EVENT_NAME":        "pull_request",
		"GITHUB_WORKFLOW_REF":      "o/r/.github/workflows/review.yml@refs/pull/3/merge",
		"GITHUB_OUTPUT":            f.output,
		"GITHUB_ENV":               f.env,
		"GITHUB_STEP_SUMMARY":      f.summary,
		"RUNNER_TEMP":              f.tmp,
		"LOUPE_CI_EVENT_ACTION":    "opened",
		"LOUPE_CI_PULL_REQUEST":    "3",
		"LOUPE_CI_SOURCE":          "loupe-ci",
		"LOUPE_CI_WAIT_FOR_CHECKS": "1",
		"LOUPE_CI_POLL_SECONDS":    "0",
		"LOUPE_CI_GRACE_SECONDS":   "0",
	}
	return f
}

func (f *captureFixture) captureResult(t *testing.T, previous map[string]any, exit int) {
	t.Helper()
	result := map[string]any{"loupe": 1, "ok": true, "command": "capture", "run": "o/r#3@2",
		"target": map[string]any{"owner": "o", "repo": "r", "number": 3, "headSha": f.repo.HeadSHA()}, "previous": previous}
	f.writeCapture(t, result, exit)
}

func (f *captureFixture) writeCapture(t *testing.T, result map[string]any, exit int) {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.stub, "capture.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.stub, "capture.exit"), []byte{byte('0' + exit)}, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *captureFixture) run(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.Join(repoRoot(t), "ci", "capture", "capture.sh"))
	cmd.Dir = f.repo.Dir
	// A git hook running the suite sets GIT_DIR, which would point the script's git at loupe's own repository.
	cmd.Env = gitenv.WithoutRepo(os.Environ())
	for k, v := range f.vars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	return out.String(), errOut.String(), err
}

func (f *captureFixture) mustRun(t *testing.T) (outputs map[string]string, stdout string) {
	t.Helper()
	stdout, stderr, err := f.run(t)
	if err != nil {
		t.Fatalf("capture.sh failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	return f.outputs(t), stdout
}

func (f *captureFixture) outputs(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, line := range strings.Split(readOptional(t, f.output), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = v
		}
	}
	return out
}

func (f *captureFixture) captured() bool {
	_, err := os.Stat(filepath.Join(f.stub, "capture.env"))
	return err == nil
}

func readOptional(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCaptureScriptSkipsAFork(t *testing.T) {
	for name, owner := range map[string]string{"fork": "forker", "deleted fork": ""} {
		t.Run(name, func(t *testing.T) {
			f := newCaptureFixture(t)
			f.gh.SetPR("o", "r", github.PullRequest{Number: 3, State: "open", HeadSHA: f.repo.HeadSHA(), HeadOwner: owner, HeadRepo: "r"})
			// A requested round still skips: a fork's token cannot publish however the round was asked for.
			f.vars["LOUPE_CI_EVENT_ACTION"] = "labeled"

			out, _ := f.mustRun(t)
			if out["skip"] != "true" || out["reason"] != "fork" {
				t.Fatalf("outputs %v, want skip=true reason=fork", out)
			}
			if f.captured() {
				t.Error("a fork's pull request MUST NOT be captured")
			}
			if summary := readOptional(t, f.summary); !strings.Contains(summary, "fork") {
				t.Errorf("job summary does not give the reason:\n%s", summary)
			}
		})
	}
}

func TestCaptureScriptSkipsAnAutomaticRoundAtAReviewedHead(t *testing.T) {
	f := newCaptureFixture(t)
	f.captureResult(t, map[string]any{"from": "github", "round": 1, "head": f.repo.HeadSHA()}, 0)

	out, _ := f.mustRun(t)
	if out["skip"] != "true" || out["reason"] != "already-reviewed" {
		t.Fatalf("outputs %v, want skip=true reason=already-reviewed", out)
	}
	if summary := readOptional(t, f.summary); !strings.Contains(summary, "already-reviewed") {
		t.Errorf("job summary does not give the reason:\n%s", summary)
	}
	if _, err := os.Stat(filepath.Join(f.repo.Dir, "review")); !os.IsNotExist(err) {
		t.Errorf("a skipped round MUST NOT lay out a review directory: %v", err)
	}
}

func TestCaptureScriptRunsARequestedRoundAtAReviewedHead(t *testing.T) {
	f := newCaptureFixture(t)
	f.vars["LOUPE_CI_EVENT_ACTION"] = "labeled"
	f.captureResult(t, map[string]any{"from": "github", "round": 1, "head": f.repo.HeadSHA()}, 0)

	out, stdout := f.mustRun(t)
	if out["skip"] != "false" || out["reason"] != "" {
		t.Fatalf("outputs %v, want skip=false", out)
	}
	if strings.Contains(stdout, "has finished") {
		t.Errorf("a requested round MUST NOT wait for checks:\n%s", stdout)
	}
	f.assertReviewed(t, out)
}

func TestCaptureScriptRunsWithoutAPreviousHead(t *testing.T) {
	f := newCaptureFixture(t)
	f.captureResult(t, map[string]any{"from": "github", "round": 1}, 0)

	out, _ := f.mustRun(t)
	if out["skip"] != "false" {
		t.Fatalf("outputs %v, want skip=false", out)
	}
	f.assertReviewed(t, out)
}

// assertReviewed checks a round that runs: its outputs, environment and review directory, and that the token reached
// loupe capture alone.
func (f *captureFixture) assertReviewed(t *testing.T, out map[string]string) {
	t.Helper()
	// The script prints its working directory, which sh resolves when the inherited PWD names another directory: on
	// macOS the temporary directory is under the /var symlink.
	dir, err := filepath.EvalSymlinks(f.repo.Dir)
	if err != nil {
		t.Fatal(err)
	}
	review := filepath.Join(dir, "review")
	want := map[string]string{"run": "o/r#3@2", "head": f.repo.HeadSHA(), "review_dir": review}
	for k, v := range want {
		if out[k] != v {
			t.Errorf("output %s = %q, want %q", k, out[k], v)
		}
	}
	env := readOptional(t, f.env)
	for _, line := range []string{"LOUPE_HOME=" + filepath.Join(f.tmp, "loupe-home"), "LOUPE_RUN=o/r#3@2"} {
		if !strings.Contains(env, line+"\n") {
			t.Errorf("GITHUB_ENV lacks %q:\n%s", line, env)
		}
	}
	if added := mustRead(t, filepath.Join(review, "src", "added.go")); !strings.Contains(added, "Added = true") {
		t.Errorf("review/ does not hold the head's files: %q", added)
	}
	if diff := mustRead(t, filepath.Join(review, "pr.diff")); !strings.HasPrefix(diff, "diff --git") {
		t.Errorf("review/pr.diff is %q", diff)
	}
	if _, err := os.Stat(filepath.Join(review, "checks.json")); err != nil {
		t.Errorf("review/checks.json: %v", err)
	}

	captureEnv := mustRead(t, filepath.Join(f.stub, "capture.env"))
	header := "AUTHORIZATION: basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+captureToken))
	for _, line := range []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=" + header, "LOUPE_HOME=" + filepath.Join(f.tmp, "loupe-home")} {
		if !strings.Contains(captureEnv, line+"\n") {
			t.Errorf("loupe capture's environment lacks %q", line)
		}
	}
	calls := mustRead(t, filepath.Join(f.stub, "calls"))
	if !strings.Contains(calls, "capture https://github.com/o/r/pull/3 --source loupe-ci --json\n") {
		t.Errorf("loupe calls:\n%s", calls)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + captureToken))
	for name, path := range map[string]string{".git/config": filepath.Join(f.repo.Dir, ".git", "config"),
		"GITHUB_ENV": f.env, "GITHUB_OUTPUT": f.output} {
		if text := readOptional(t, path); strings.Contains(text, captureToken) || strings.Contains(text, encoded) {
			t.Errorf("%s holds the token:\n%s", name, text)
		}
	}
}

func TestCaptureScriptWaitsForOtherChecksOnly(t *testing.T) {
	f := newCaptureFixture(t)
	head := f.repo.HeadSHA()
	f.gh.SeedWorkflowRun("o", "r", fakegh.WorkflowRun{ID: 7, Path: ".github/workflows/review.yml", HeadSHA: head})
	f.gh.SeedWorkflowRun("o", "r", fakegh.WorkflowRun{ID: 8, Path: ".github/workflows/ci.yml", HeadSHA: head})
	// The round's own job is still running while it waits; counting it would wait out the whole timeout.
	f.gh.SeedCheckRun("o", "r", head, fakegh.CheckRun{Name: "review / review", Status: "in_progress",
		DetailsURL: "https://github.com/o/r/actions/runs/7/job/70"})
	f.gh.SeedCheckRun("o", "r", head, fakegh.CheckRun{Name: "test", Status: "completed", Conclusion: "success",
		DetailsURL: "https://github.com/o/r/actions/runs/8/job/80"})
	f.gh.SeedCheckRun("o", "r", head, fakegh.CheckRun{Name: "codecov", Status: "completed", Conclusion: "failure"})

	out, stdout := f.mustRun(t)
	if out["skip"] != "false" {
		t.Fatalf("outputs %v", out)
	}
	if !strings.Contains(stdout, "every other check on "+head+" has finished") {
		t.Errorf("the wait did not run or did not finish:\n%s", stdout)
	}
	var checks []map[string]any
	if err := json.Unmarshal([]byte(mustRead(t, filepath.Join(f.repo.Dir, "review", "checks.json"))), &checks); err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{{"name": "test", "conclusion": "success"}, {"name": "codecov", "conclusion": "failure"}}
	if got, _ := json.Marshal(checks); string(got) != mustJSON(t, want) {
		t.Errorf("checks.json %s, want %s", got, mustJSON(t, want))
	}
}

// GitHub lists at most 100 check runs or workflow runs a page, so a busy commit spills onto later pages.
func TestCaptureScriptReadsEveryPage(t *testing.T) {
	f := newCaptureFixture(t)
	f.gh.SetPageSize(1)
	head := f.repo.HeadSHA()
	f.gh.SeedWorkflowRun("o", "r", fakegh.WorkflowRun{ID: 8, Path: ".github/workflows/ci.yml", HeadSHA: head})
	f.gh.SeedWorkflowRun("o", "r", fakegh.WorkflowRun{ID: 7, Path: ".github/workflows/review.yml", HeadSHA: head})
	f.gh.SeedCheckRun("o", "r", head, fakegh.CheckRun{Name: "test", Status: "completed", Conclusion: "success",
		DetailsURL: "https://github.com/o/r/actions/runs/8/job/80"})
	f.gh.SeedCheckRun("o", "r", head, fakegh.CheckRun{Name: "review / review", Status: "completed", Conclusion: "success",
		DetailsURL: "https://github.com/o/r/actions/runs/7/job/70"})
	f.gh.SeedCheckRun("o", "r", head, fakegh.CheckRun{Name: "codecov", Status: "completed", Conclusion: "failure"})

	f.mustRun(t)
	var checks []map[string]any
	if err := json.Unmarshal([]byte(mustRead(t, filepath.Join(f.repo.Dir, "review", "checks.json"))), &checks); err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{{"name": "test", "conclusion": "success"}, {"name": "codecov", "conclusion": "failure"}}
	if got, _ := json.Marshal(checks); string(got) != mustJSON(t, want) {
		t.Errorf("checks.json %s, want %s", got, mustJSON(t, want))
	}
}

// Real check runs are large objects, and pages that together pass Linux's 128 KiB limit on one argument MUST still
// combine.
func TestCaptureScriptReadsPagesLargerThanAnArgument(t *testing.T) {
	f := newCaptureFixture(t)
	head := f.repo.HeadSHA()
	const runs = 150
	for i := range runs {
		f.gh.SeedCheckRun("o", "r", head, fakegh.CheckRun{Name: fmt.Sprintf("check-%03d-%s", i, strings.Repeat("x", 2048)),
			Status: "completed", Conclusion: "success"})
	}

	f.mustRun(t)
	var checks []map[string]any
	if err := json.Unmarshal([]byte(mustRead(t, filepath.Join(f.repo.Dir, "review", "checks.json"))), &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks) != runs {
		t.Errorf("checks.json holds %d check runs, want %d", len(checks), runs)
	}
}

// A caller whose review job lacks actions: read cannot list its own runs. That costs the wait and checks.json, as a
// failed check-run listing does, not the round.
func TestCaptureScriptReviewsWhenItCannotListItsOwnRuns(t *testing.T) {
	f := newCaptureFixture(t)
	f.gh.Fail("GET", "/repos/o/r/actions/runs", 403)
	f.gh.SeedCheckRun("o", "r", f.repo.HeadSHA(), fakegh.CheckRun{Name: "test", Status: "in_progress"})

	out, stdout := f.mustRun(t)
	if out["skip"] != "false" {
		t.Fatalf("outputs %v", out)
	}
	if !strings.Contains(stdout, "::warning::") || !strings.Contains(stdout, "actions: read") {
		t.Errorf("no warning naming actions: read:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(f.repo.Dir, "review", "checks.json")); !os.IsNotExist(err) {
		t.Errorf("review/checks.json was written without the run list: %v", err)
	}
}

// The published footer and loupe-meta name the model only when capture recorded it.
func TestCaptureScriptPassesTheModelToCapture(t *testing.T) {
	for _, c := range []struct {
		name, model string
		want        bool
	}{
		{"given", "anthropic/claude-opus-5.5", true},
		{"empty", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newCaptureFixture(t)
			f.vars["LOUPE_CI_MODEL"] = c.model
			f.mustRun(t)
			calls := mustRead(t, filepath.Join(f.stub, "calls"))
			if got := strings.Contains(calls, "--model anthropic/claude-opus-5.5"); got != c.want {
				t.Errorf("loupe calls:\n%s\nwant --model passed: %v", calls, c.want)
			}
			if !c.want && strings.Contains(calls, "--model") {
				t.Errorf("an empty model still reached capture:\n%s", calls)
			}
		})
	}
}

// One fast check finishing says nothing about the slower workflows that have not registered theirs yet.
func TestCaptureScriptWaitsOutTheGraceBeforeAcceptingFinished(t *testing.T) {
	f := newCaptureFixture(t)
	// The script counts whole seconds, so a 3s grace guarantees more than 2s of real time.
	f.vars["LOUPE_CI_GRACE_SECONDS"] = "3"
	f.vars["LOUPE_CI_POLL_SECONDS"] = "1"
	f.gh.SeedCheckRun("o", "r", f.repo.HeadSHA(), fakegh.CheckRun{Name: "lint", Status: "completed", Conclusion: "success"})

	start := time.Now()
	f.mustRun(t)
	if elapsed := time.Since(start); elapsed < 2*time.Second {
		t.Errorf("the wait accepted the finished checks after %s, before the 3s grace", elapsed)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCaptureScriptFailsOnARefusal(t *testing.T) {
	f := newCaptureFixture(t)
	f.writeCapture(t, map[string]any{"loupe": 1, "ok": false, "command": "capture",
		"error": map[string]any{"code": "pr", "message": "pull request o/r#3 was not found on github.com",
			"fix": "check that o/r has pull request 3"}}, 1)

	_, stderr, err := f.run(t)
	if err == nil {
		t.Fatal("a refused capture MUST fail the step")
	}
	for _, line := range []string{"error: pull request o/r#3 was not found on github.com\n",
		"fix: check that o/r has pull request 3\n"} {
		if !strings.Contains(stderr, line) {
			t.Errorf("stderr lacks %q:\n%s", line, stderr)
		}
	}
	if out := f.outputs(t); out["skip"] == "true" {
		t.Errorf("a refusal is not a skip: %v", out)
	}
}

func TestCaptureScriptRefusesABadPullRequestNumber(t *testing.T) {
	f := newCaptureFixture(t)
	f.vars["LOUPE_CI_PULL_REQUEST"] = "3\nskip=true"

	_, stderr, err := f.run(t)
	if err == nil || !strings.Contains(stderr, "pull_request") {
		t.Fatalf("err %v, stderr:\n%s", err, stderr)
	}
	if f.captured() || readOptional(t, f.output) != "" {
		t.Error("a bad number MUST stop the script before it captures or sets an output")
	}
}
