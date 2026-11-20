package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/secondshift-labs/loupe/internal/draft"
	"github.com/secondshift-labs/loupe/internal/refusal"
)

func TestSummaryFlagRefusals(t *testing.T) {
	home, _ := sendBackRun(t)
	checkRefusals(t, home, []refusalCase{
		{"an unknown --by", "", []string{"summary", "--run", "o/r#1", "--body", "x", "--by", "bot"}, refusal.Usage, `--by must be agent or human, not "bot"`},
		{"no summary", "", []string{"summary", "--run", "o/r#1", "--expect-findings", "2"}, refusal.Usage, "no summary given"},
		{"an agent without --expect-findings", "", []string{"summary", "--run", "o/r#1", "--body", "Nothing found."}, refusal.Usage, "--expect-findings is required with --by agent"},
		{"--by agent without --expect-findings", "", []string{"summary", "--run", "o/r#1", "--body", "Nothing found.", "--by", "agent"}, refusal.Usage, "--expect-findings is required with --by agent"},
		{"--from with --body", `{"summary": "x"}`, []string{"summary", "--run", "o/r#1", "--from", "-", "--body", "x", "--expect-findings", "2"}, refusal.Usage, "--from cannot be combined with --body"},
		{"no run", "", []string{"summary", "--run", "o/r#9", "--expect-findings", "2", "--body", "x"}, refusal.NoRun, "o/r#9"},
		{"malformed input", "{", []string{"summary", "--run", "o/r#1", "--expect-findings", "2", "--from", "-"}, refusal.Input, "input ends at byte 1"},
		{"input without a summary", `{}`, []string{"summary", "--run", "o/r#1", "--expect-findings", "2", "--from", "-"}, refusal.Input, `input has no "summary" field`},
	})
}

func TestSummaryFromInputSaysWhatItSet(t *testing.T) {
	home, dir := sendBackRun(t)
	code, s := runWithStdin(t, home, `{"summary": "Two findings."}`, "summary", "--run", "o/r#1", "--expect-findings", "2", "--from", "-")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if want := "+ Summary set on o/r#1@1 with 2 included findings  draft version 1\n"; s.stdout.String() != want {
		t.Fatalf("stdout %q, want %q", s.stdout.String(), want)
	}
	if d, err := draft.Load(dir); err != nil || d.Summary != "Two findings." {
		t.Fatalf("summary %q err %v", d.Summary, err)
	}
}

func TestSummaryByHumanNeedsNoExpectedCount(t *testing.T) {
	home, dir := sendBackRun(t)
	if code, s := runWithStdin(t, home, "", "summary", "--run", "o/r#1", "--body", "Nothing found.", "--by", "human"); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if d, err := draft.Load(dir); err != nil || d.Summary != "Nothing found." {
		t.Fatalf("summary %q err %v", d.Summary, err)
	}
}

func TestSummaryVerdictRefusals(t *testing.T) {
	home, dir := sendBackRun(t)
	before, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	const values = "comment, approve or request-changes"
	checkRefusals(t, home, []refusalCase{
		{"--verdict with --from", `{"summary": "x"}`, []string{"summary", "--run", "o/r#1", "--from", "-", "--verdict", "approve", "--expect-findings", "2"}, refusal.Usage, "--from cannot be combined with --verdict"},
		{"an unknown --verdict", "", []string{"summary", "--run", "o/r#1", "--body", "x", "--verdict", "maybe", "--expect-findings", "2"}, refusal.Usage, `--verdict "maybe" is not one of ` + values},
		{"an unknown input verdict", `{"summary": "x", "verdict": "maybe"}`, []string{"summary", "--run", "o/r#1", "--from", "-", "--expect-findings", "2"}, refusal.Input, `input field "verdict" is "maybe", not one of ` + values},
		{"--verdict by a human", "", []string{"summary", "--run", "o/r#1", "--body", "x", "--verdict", "approve", "--by", "human"}, refusal.Usage, "the verdict is the agent's recommendation"},
		{"an input verdict by a human", `{"summary": "x", "verdict": "approve"}`, []string{"summary", "--run", "o/r#1", "--from", "-", "--by", "human"}, refusal.Usage, "the verdict is the agent's recommendation"},
	})
	after, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version || after.Verdict != "" {
		t.Fatalf("version %d → %d, verdict %q", before.Version, after.Version, after.Verdict)
	}
}

func TestSummaryVerdictRefusalFixes(t *testing.T) {
	home, _ := sendBackRun(t)
	for _, c := range []struct {
		stdin string
		args  []string
		fix   string
	}{
		{"", []string{"--body", "x", "--verdict", "maybe", "--expect-findings", "2"}, "use one of comment, approve or request-changes"},
		{`{"summary": "x", "verdict": "maybe"}`, []string{"--from", "-", "--expect-findings", "2"}, "use one of comment, approve or request-changes"},
		{"", []string{"--body", "x", "--verdict", "approve", "--by", "human"}, "drop --verdict; the human picks the action in loupe review"},
	} {
		_, s := runWithStdin(t, home, c.stdin, append([]string{"summary", "--run", "o/r#1", "--json"}, c.args...)...)
		if got := errorOf(decodeOne(t, s.stdout.Bytes()))["fix"]; got != c.fix {
			t.Errorf("%v: fix %q, want %q", c.args, got, c.fix)
		}
	}
}

func TestSummaryVerdictOnALocalRun(t *testing.T) {
	home, dir := localRun(t, "changed", localFinding("f-001"))
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	errObj := wantCode(t, Execute(deps, []string{"summary", "--run", "local#1", "--body", "x", "--verdict", "approve", "--expect-findings", "1", "--json"}), s, refusal.WrongMode)
	if errObj["message"] != "loupe summary --verdict applies only to pull request runs" {
		t.Fatalf("error %v", errObj)
	}
	code, s := runWithStdin(t, home, `{"summary": "x", "verdict": "approve"}`, "summary", "--run", "local#1", "--from", "-", "--expect-findings", "1", "--json")
	errObj = wantCode(t, code, s, refusal.Input)
	if errObj["message"] != `input field "verdict" does not apply to local runs` || errObj["fix"] != `drop "verdict" from the input` {
		t.Fatalf("error %v", errObj)
	}
	if d, err := draft.Load(dir); err != nil || d.Version != 0 || d.Summary != "" {
		t.Fatalf("draft changed: version %d summary %q err %v", d.Version, d.Summary, err)
	}
}

// A summary call states the agent's whole view of the round, so one without a verdict clears the stored one.
func TestSummaryStoresAndClearsTheVerdict(t *testing.T) {
	home, dir := sendBackRun(t)
	verdict := func() string {
		t.Helper()
		d, err := draft.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		return d.Verdict
	}
	for _, c := range []struct {
		stdin string
		args  []string
		want  string
	}{
		{"", []string{"--body", "x", "--verdict", "approve"}, "approve"},
		{"", []string{"--body", "y"}, ""},
		{`{"summary": "x", "verdict": "request-changes"}`, []string{"--from", "-"}, "request-changes"},
		{`{"summary": "y"}`, []string{"--from", "-"}, ""},
	} {
		code, s := runWithStdin(t, home, c.stdin, append([]string{"summary", "--run", "o/r#1", "--expect-findings", "2"}, c.args...)...)
		if code != 0 {
			t.Fatalf("%v: exit %d stderr %q", c.args, code, s.stderr.String())
		}
		if got := verdict(); got != c.want {
			t.Fatalf("%v: verdict %q, want %q", c.args, got, c.want)
		}
	}
}

// The input's verdict is checked against the run's kind, so a target that no longer reads refuses before any write.
func TestSummaryInputVerdictRefusesAnUnreadableTarget(t *testing.T) {
	home, dir := sendBackRun(t)
	if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, s := runWithStdin(t, home, `{"summary": "x", "verdict": "approve"}`, "summary", "--run", "o/r#1", "--from", "-", "--expect-findings", "2", "--json")
	wantCode(t, code, s, refusal.Record)
}

func TestSummaryByHumanKeepsTheAgentsVerdict(t *testing.T) {
	home, dir := sendBackRun(t)
	for _, args := range [][]string{
		{"--body", "x", "--verdict", "approve", "--expect-findings", "2"},
		{"--body", "Reworded by hand.", "--by", "human"},
	} {
		if code, s := runWithStdin(t, home, "", append([]string{"summary", "--run", "o/r#1"}, args...)...); code != 0 {
			t.Fatalf("%v: exit %d stderr %q", args, code, s.stderr.String())
		}
	}
	if d, err := draft.Load(dir); err != nil || d.Verdict != "approve" || d.Summary != "Reworded by hand." {
		t.Fatalf("summary %q verdict %q err %v", d.Summary, d.Verdict, err)
	}
}
