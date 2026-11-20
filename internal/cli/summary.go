package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/secondshift-labs/loupe/internal/draft"
	"github.com/secondshift-labs/loupe/internal/mode"
	"github.com/secondshift-labs/loupe/internal/publish"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/run"
)

const summaryHelp = `Set the summary the human reads while sorting findings.

The summary orients whoever decides the findings: what kind of review this is, what was looked
at, what could not be checked. An attended publication does not post it. The review's own
opening prose is written by the human at the publish confirmation, in their words, so write the
summary for the one person who reads it before deciding, not for the pull request's author.
An unattended publication has no such person, and there the summary is the review's opening.

Input (--from <file>, or --from - for stdin):

  {"summary": "Markdown", "verdict": "approve"}

--body <text> sets the summary directly and cannot be combined with --from. The summary must
pass the Markdown allowlist and may be empty. On every run it refuses a finding id, f- and
three or more digits such as f-002, with markdown and details.rule finding-id: name an earlier
finding by its subject. A fence or an http or https URL is exempt; a code span is not.

--expect-findings <n> is required with --by agent (the default) and optional with --by human.
Inside the run lock it compares n with the number of included findings; on a mismatch the
summary is left unchanged and the refusal lists details.included as [{"id", "title"}], so an
agent learns which of its findings did not land.

--verdict <action>, or "verdict" in the input, is the action you would publish with: comment,
approve or request-changes. It sets only where the publish step of loupe review starts, and
the human's decisions override it there: an accepted blocking finding turns approve into
request-changes, and request-changes with no blocking finding left turns into comment. It never
sets the action that is published. Each call sets the summary and the verdict together, so a
call without a verdict clears the one before. A local run takes no verdict, and neither does
--by human.

Result (--json):
  {"loupe": 1, "ok": true, "command": "summary", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "version": 5,
   "includedCount": 2}`

func newSummaryCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "summary",
		Short:   "Set the summary the human reads while sorting findings",
		Long:    summaryHelp,
		Example: "  loupe summary --run owner/repo#123 --from summary.json --expect-findings 2 --json\n  loupe summary --body \"Two issues, one blocking.\" --expect-findings 2 --verdict request-changes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSummary(cmd, deps)
		},
	}
	cmd.Flags().String("from", "", "read JSON input from a file, or from stdin with -")
	cmd.Flags().String("body", "", "summary text (Markdown)")
	cmd.Flags().Int("expect-findings", 0, "refuse unless exactly this many findings are included")
	cmd.Flags().String("verdict", "", "the action you would publish with: comment, approve or request-changes")
	addMutationFlags(cmd)
	return cmd
}

type summaryInput struct {
	Summary *string `json:"summary"`
	Verdict *string `json:"verdict"`
}

var verdictValues = strings.Join(publish.Actions[:len(publish.Actions)-1], ", ") + " or " + publish.Actions[len(publish.Actions)-1]

var verdictFix = "use one of " + verdictValues

// humanVerdict refuses a verdict from the human, because review labels it as the agent's recommendation.
func humanVerdict() error {
	return refusal.New(refusal.Usage, "the verdict is the agent's recommendation and cannot be set with --by human",
		"drop --verdict; the human picks the action in loupe review")
}

func runSummary(cmd *cobra.Command, deps Deps) error {
	if err := ConflictsWithFrom(cmd, "body", "verdict"); err != nil {
		return err
	}
	by, expectVersion, err := mutationOptions(cmd)
	if err != nil {
		return err
	}
	var expectFindings *int
	if cmd.Flags().Changed("expect-findings") {
		n, _ := cmd.Flags().GetInt("expect-findings")
		expectFindings = &n
	} else if by == draft.ByAgent {
		return refusal.New(refusal.Usage, "--expect-findings is required with --by agent",
			"loupe summary --expect-findings <n> with n the number of findings you filed")
	}
	verdict, _ := cmd.Flags().GetString("verdict")
	if cmd.Flags().Changed("verdict") {
		if by == draft.ByHuman {
			return humanVerdict()
		}
		if !slices.Contains(publish.Actions, verdict) {
			return refusal.New(refusal.Usage, fmt.Sprintf("--verdict %q is not one of %s", verdict, verdictValues), verdictFix)
		}
	}
	if !cmd.Flags().Changed("from") && !cmd.Flags().Changed("body") {
		return refusal.New(refusal.Usage, "no summary given", "pass --body <text> or --from <file>|-")
	}
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	summary, _ := cmd.Flags().GetString("body")
	if cmd.Flags().Changed("from") {
		from, _ := cmd.Flags().GetString("from")
		var in summaryInput
		if err := DecodeInput("summary", from, deps.Stdin, &in); err != nil {
			return err
		}
		if in.Summary == nil {
			return refusal.New(refusal.Input, `input has no "summary" field`, "see loupe summary --help for the input shape")
		}
		summary = *in.Summary
		if in.Verdict != nil {
			if err := checkInputVerdict(dir, *in.Verdict, by); err != nil {
				return err
			}
			verdict = *in.Verdict
		}
	}
	d, err := draft.Mutate(dir, "summary", expectVersion, deps.Getenv, func(d *draft.Draft) error {
		return draft.SetSummary(d, summary, verdict, expectFindings, by)
	})
	if err != nil {
		return err
	}
	included := draft.IncludedCount(d)
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &d.Version, map[string]any{"includedCount": included})
	}
	s := deps.outStyle()
	return printDone(deps, fmt.Sprintf("Summary set on %s with %d included %s", s.Accent.Render(ref.String()), included, plural(included, "finding")), d.Version)
}

func checkInputVerdict(dir, verdict, by string) error {
	target, err := run.LoadTarget(dir)
	if err != nil {
		return err
	}
	switch {
	case mode.Of(target).InputRules().RefuseVerdict:
		return refusal.New(refusal.Input, `input field "verdict" does not apply to local runs`, `drop "verdict" from the input`)
	case by == draft.ByHuman:
		return humanVerdict()
	case !slices.Contains(publish.Actions, verdict):
		return refusal.New(refusal.Input, fmt.Sprintf(`input field "verdict" is %q, not one of %s`, verdict, verdictValues), verdictFix)
	}
	return nil
}
