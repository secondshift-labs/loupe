package publish

import (
	"context"
	"strings"
	"testing"

	"github.com/secondshift-labs/loupe/internal/draft"
	"github.com/secondshift-labs/loupe/internal/github"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/run"
	"github.com/secondshift-labs/loupe/internal/testutil/fakegh"
	"github.com/secondshift-labs/loupe/internal/testutil/refusaltest"
)

func newFake(t *testing.T) (*fakegh.Server, github.Client) {
	t.Helper()
	gh := fakegh.New(t)
	gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, URL: prLink, Title: "Add widgets", State: "open", Author: "author",
		BaseRef: "main", BaseSHA: "2222222222222222222222222222222222222222", HeadSHA: headSHA})
	gh.SetViewer("reviewer")
	return gh, gh.Client(t)
}

// tokenKindClient overrides TokenKind on a real fake client, since fakegh always builds a user-kind token.
type tokenKindClient struct {
	github.Client
	kind github.TokenKind
}

func (c tokenKindClient) TokenKind() github.TokenKind { return c.kind }

// assertOnlyCreateWrites fails if the fake received any write other than creating a review or editing one's body.
func assertOnlyCreateWrites(t *testing.T, gh *fakegh.Server) {
	t.Helper()
	for _, r := range gh.Requests() {
		create := r.Method == "POST" && r.Path == "/repos/acme/widgets/pulls/42/reviews"
		edit := r.Method == "PUT" && strings.HasPrefix(r.Path, "/repos/acme/widgets/pulls/42/reviews/")
		if r.Method != "GET" && !create && !edit {
			t.Errorf("unexpected write %s %s", r.Method, r.Path)
		}
	}
}

// gateErr is Gates for a test that only cares whether it refused.
func gateErr(ctx context.Context, in GateInput) error {
	_, err := Gates(ctx, in)
	return err
}

// wantRefusal returns the refusal message.
func wantRefusal(t *testing.T, err error, code refusal.Code, fixParts ...string) string {
	t.Helper()
	r := refusaltest.Want(t, err, code)
	for _, part := range fixParts {
		if !strings.Contains(r.Fix, part) {
			t.Errorf("%s fix %q lacks %q", code, r.Fix, part)
		}
	}
	return r.Message
}

func TestGatesRunInOrder(t *testing.T) {
	gh, client := newFake(t)
	ctx := context.Background()

	// Every gate fails at first; each step clears the gate that refused and expects the next one.
	d := readyDraft()
	gh.SetHead("acme", "widgets", 42, "3333333333333333333333333333333333333333")
	gh.SetViewer("author")
	in := GateInput{IsTerminal: false, GitHub: client, Target: fixtureTarget(), Action: "approve", Draft: d}

	wantRefusal(t, gateErr(ctx, in), refusal.TTY)
	if n := len(gh.Requests()); n != 0 {
		t.Fatalf("tty gate contacted GitHub %d times", n)
	}

	in.IsTerminal = true
	wantRefusal(t, gateErr(ctx, in), refusal.HeadMoved, "loupe capture "+prLink)

	gh.SetHead("acme", "widgets", 42, headSHA)
	wantRefusal(t, gateErr(ctx, in), refusal.OwnPR, "--action comment")
	in.Action = "request-changes"
	wantRefusal(t, gateErr(ctx, in), refusal.OwnPR, "--action comment")

	gh.SetViewer("reviewer")
	in.Action = "approve"
	msg := wantRefusal(t, gateErr(ctx, in), refusal.Blocking, "--action comment", "--action request-changes", "exclude", "unblock", "loupe review")
	if !strings.Contains(msg, "f-001") || strings.Contains(msg, "f-003") || strings.Contains(msg, "f-004") {
		t.Errorf("blocking message %q", msg)
	}

	in.Action = "comment"
	empty := draft.NewEmpty()
	empty.Findings = []draft.Finding{finding("f-001", "issue", false, nil)}
	empty.Decisions["f-001"] = draft.Decision{FindingID: "f-001", Decision: draft.DecisionExcluded, FindingRev: 1, At: fixtureNow}
	in.Draft = empty
	wantRefusal(t, gateErr(ctx, in), refusal.Empty, "loupe add", "loupe summary")

	// A pending finding is publishable, so the draft is not empty, only not ready. The excluded one above
	// carries the same included flag and does not count.
	delete(empty.Decisions, "f-001")
	wantRefusal(t, gateErr(ctx, in), refusal.NotReady, "loupe review")

	empty.Summary = "Summary only."
	wantRefusal(t, gateErr(ctx, in), refusal.NotReady, "loupe review")

	accept(empty, "f-001")
	empty.Notes = []draft.Note{{ID: "n-001", FindingID: "f-001", Body: "why?", At: fixtureNow, Status: draft.NoteOpen}}
	wantRefusal(t, gateErr(ctx, in), refusal.NotReady, "loupe review")

	empty.Notes[0].Status = draft.NoteResolved
	if err := gateErr(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.Draft = readyDraft()
	if err := gateErr(ctx, in); err != nil {
		t.Fatalf("ready draft with comment: %v", err)
	}
	assertOnlyCreateWrites(t, gh)
	if gh.CreateCount() != 0 {
		t.Fatalf("create count %d", gh.CreateCount())
	}
}

func TestGatesAllowCommentOnOwnPullRequest(t *testing.T) {
	gh, client := newFake(t)
	gh.SetViewer("author")
	if err := gateErr(context.Background(), GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "comment", Draft: readyDraft()}); err != nil {
		t.Fatal(err)
	}
}

func TestGatesSummaryOnlyDraftIsNotEmpty(t *testing.T) {
	_, client := newFake(t)
	d := draft.NewEmpty()
	d.Summary = "Nothing else to say."
	if err := gateErr(context.Background(), GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "approve", Draft: d}); err != nil {
		t.Fatal(err)
	}
}

func TestGatesBlockingCountsPendingFindings(t *testing.T) {
	_, client := newFake(t)
	d := draft.NewEmpty()
	d.Findings = []draft.Finding{finding("f-001", "issue", true, nil)}
	msg := wantRefusal(t, gateErr(context.Background(), GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "approve", Draft: d}),
		refusal.Blocking, "--action comment")
	if !strings.Contains(msg, "f-001") {
		t.Fatalf("blocking message %q", msg)
	}

	d.Decisions["f-001"] = draft.Decision{FindingID: "f-001", Decision: draft.DecisionExcluded, FindingRev: 1, At: fixtureNow}
	d.Summary = "Excluded the blocker."
	if err := gateErr(context.Background(), GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "approve", Draft: d}); err != nil {
		t.Fatalf("excluded blocking finding: %v", err)
	}
}

func TestGatesRefusesTokenBeforeAnyCall(t *testing.T) {
	gh, client := newFake(t)
	in := GateInput{Unattended: true, GitHub: client, Target: fixtureTarget(), Action: "comment", Draft: readyDraft()}
	wantRefusal(t, gateErr(context.Background(), in), refusal.Token, "GITHUB_TOKEN", "permissions: pull-requests: write", "contents: read")
	if n := len(gh.Requests()); n != 0 {
		t.Fatalf("token gate contacted GitHub %d times", n)
	}
}

func TestGatesRefusesInstallationTokenWhenAttended(t *testing.T) {
	_, real := newFake(t)
	client := tokenKindClient{Client: real, kind: github.Installation}
	in := GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "comment", Draft: readyDraft()}
	wantRefusal(t, gateErr(context.Background(), in), refusal.Token, "--unattended")
}

func TestGatesUnattendedSkipsTTYOwnPRAndReadinessButKeepsHeadMovedAndEmpty(t *testing.T) {
	gh, real := newFake(t)
	client := tokenKindClient{Client: real, kind: github.Installation}
	// The viewer and the pending finding would refuse as own-pr and not-ready if those gates still ran.
	gh.SetViewer("author")
	d := readyDraft()
	delete(d.Decisions, "f-002")
	target := fixtureTarget()
	// An installation-captured run.
	target.Viewer = ""
	in := GateInput{Unattended: true, IsTerminal: false, GitHub: client, Target: target, Action: "comment", Draft: d}
	if err := gateErr(context.Background(), in); err != nil {
		t.Fatalf("unattended gates: %v", err)
	}

	gh.SetHead("acme", "widgets", 42, "3333333333333333333333333333333333333333")
	wantRefusal(t, gateErr(context.Background(), in), refusal.HeadMoved, "loupe capture "+prLink)
	gh.SetHead("acme", "widgets", 42, headSHA)

	in.Draft = draft.NewEmpty()
	wantRefusal(t, gateErr(context.Background(), in), refusal.Empty, "loupe add", "loupe summary")
}

func TestGatesRefusesTokenKindDifferentFromCapture(t *testing.T) {
	// newFake's client is user-kind, and fixtureTarget's Viewer is the login a user token captured with.
	_, real := newFake(t)
	installation := tokenKindClient{Client: real, kind: github.Installation}

	userCaptured := fixtureTarget()
	wantRefusal(t, gateErr(context.Background(), GateInput{Unattended: true, GitHub: installation, Target: userCaptured, Action: "comment", Draft: readyDraft()}),
		refusal.Viewer, "capture and publish with the same token kind")

	installationCaptured := fixtureTarget()
	installationCaptured.Viewer = ""
	wantRefusal(t, gateErr(context.Background(), GateInput{IsTerminal: true, GitHub: real, Target: installationCaptured, Action: "comment", Draft: readyDraft()}),
		refusal.Viewer, "capture and publish with the same token kind")
}

func TestActionRefusal(t *testing.T) {
	d := readyDraft()
	if err := ActionRefusal("comment", "author", "author", d); err != nil {
		t.Fatalf("comment on own pull request: %v", err)
	}
	wantRefusal(t, ActionRefusal("request-changes", "author", "author", d), refusal.OwnPR)
	wantRefusal(t, ActionRefusal("approve", "reviewer", "author", d), refusal.Blocking)
	if err := ActionRefusal("request-changes", "reviewer", "author", d); err != nil {
		t.Fatal(err)
	}
}

// verdictDraft holds f-001, accepted and never blocking, and f-002 and f-005 in the given states.
func verdictDraft(f002, f005 string) *draft.Draft {
	d := draft.NewEmpty()
	d.Findings = []draft.Finding{finding("f-001", "issue", false, nil)}
	accept(d, "f-001")
	for _, fs := range [][2]string{{"f-002", f002}, {"f-005", f005}} {
		id, state := fs[0], fs[1]
		if state == "" {
			continue
		}
		f := finding(id, "issue", state != "unblocked", nil)
		switch state {
		case "accepted", "unblocked":
			accept(d, id)
		case "excluded":
			d.Decisions[id] = draft.Decision{FindingID: id, Decision: draft.DecisionExcluded, FindingRev: 1, At: fixtureNow}
		case "withdrawn":
			f.Included = false
		}
		d.Findings = append(d.Findings, f)
	}
	return d
}

func TestStartAction(t *testing.T) {
	const reviewer, author = "reviewer", "author"
	cases := []struct {
		name, verdict, viewer string
		d                     *draft.Draft
		start, why            string
	}{
		{"no verdict", "", reviewer, verdictDraft("accepted", ""), "comment", ""},
		{"unknown verdict", "bogus", reviewer, verdictDraft("", ""), "comment", ""},
		{"comment", "comment", reviewer, verdictDraft("", ""), "comment", ""},
		{"comment with a blocking finding", "comment", reviewer, verdictDraft("accepted", ""), "comment", ""},
		{"approve", "approve", reviewer, verdictDraft("", ""), "approve", ""},
		{"approve with no findings", "approve", reviewer, draft.NewEmpty(), "approve", ""},
		{"approve, blocking accepted", "approve", reviewer, verdictDraft("accepted", ""), "request-changes",
			"f-002 is blocking"},
		{"approve, two blocking accepted", "approve", reviewer, verdictDraft("accepted", "accepted"), "request-changes",
			"f-002, f-005 are blocking"},
		{"approve, blocking excluded", "approve", reviewer, verdictDraft("excluded", ""), "approve", ""},
		{"approve, blocking withdrawn", "approve", reviewer, verdictDraft("withdrawn", ""), "approve", ""},
		{"approve, blocking unblocked", "approve", reviewer, verdictDraft("unblocked", ""), "approve", ""},
		{"request-changes, blocking accepted", "request-changes", reviewer, verdictDraft("accepted", ""), "request-changes", ""},
		{"request-changes, blocking excluded", "request-changes", reviewer, verdictDraft("excluded", ""), "comment",
			"no published finding is blocking"},
		{"request-changes, nothing blocking", "request-changes", reviewer, verdictDraft("", ""), "comment",
			"no published finding is blocking"},
		{"own pull request, approve", "approve", author, verdictDraft("", ""), "comment",
			"your own pull request"},
		{"own pull request, approve, blocking accepted", "approve", author, verdictDraft("accepted", ""), "comment",
			"your own pull request"},
		{"own pull request, request-changes", "request-changes", author, verdictDraft("accepted", ""), "comment",
			"your own pull request"},
		{"own pull request, request-changes, nothing blocking", "request-changes", author, verdictDraft("", ""), "comment",
			"your own pull request"},
		{"own pull request, comment", "comment", author, verdictDraft("accepted", ""), "comment", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, why := StartAction(tc.verdict, tc.viewer, author, tc.d)
			if start != tc.start || why != tc.why {
				t.Fatalf("got (%q, %q), want (%q, %q)", start, why, tc.start, tc.why)
			}
		})
	}
}

// The start is never an action the step would refuse, whatever the verdict and the decisions.
func TestStartActionIsAlwaysAvailable(t *testing.T) {
	states := []string{"", "accepted", "excluded", "withdrawn", "unblocked"}
	for _, verdict := range append([]string{"", "bogus"}, Actions...) {
		for _, f002 := range states {
			for _, f005 := range states {
				for _, viewer := range []string{"reviewer", "author"} {
					d := verdictDraft(f002, f005)
					start, _ := StartAction(verdict, viewer, "author", d)
					if err := ActionRefusal(start, viewer, "author", d); err != nil {
						t.Fatalf("verdict %q, f-002 %q, f-005 %q, viewer %s: start %q refused: %v", verdict, f002, f005, viewer, start, err)
					}
				}
			}
		}
	}
}

func TestUnavailableIsShort(t *testing.T) {
	d := readyDraft()
	for _, c := range []struct{ action, viewer, want string }{
		{"comment", "author", ""},
		{"approve", "reviewer", "f-001 is blocking"},
		{"request-changes", "reviewer", ""},
		{"approve", "author", "your own pull request"},
		{"request-changes", "author", "your own pull request"},
	} {
		if got := Unavailable(c.action, c.viewer, "author", d); got != c.want {
			t.Errorf("%s by %s: %q, want %q", c.action, c.viewer, got, c.want)
		}
	}
}

func TestBlockingPhraseStaysShort(t *testing.T) {
	for _, c := range []struct {
		ids  []string
		want string
	}{
		{[]string{"f-001"}, "f-001 is blocking"},
		{[]string{"f-001", "f-002", "f-003"}, "f-001, f-002, f-003 are blocking"},
		{[]string{"f-001", "f-002", "f-003", "f-004", "f-005"}, "f-001, f-002 and 3 more are blocking"},
	} {
		if got := blockingPhrase(c.ids); got != c.want {
			t.Errorf("%v: %q, want %q", c.ids, got, c.want)
		}
	}
}

func TestClosedSince(t *testing.T) {
	cases := []struct {
		name     string
		captured string
		pr       github.PullRequest
		want     string
	}{
		{"open", "open", github.PullRequest{State: "open"}, ""},
		{"closed", "open", github.PullRequest{State: "closed"}, "closed"},
		{"merged", "open", github.PullRequest{State: "closed", Merged: true}, "merged"},
		{"empty state", "open", github.PullRequest{}, ""},
		{"unexpected state", "open", github.PullRequest{State: "draft"}, ""},
		{"captured closed, still closed", "closed", github.PullRequest{State: "closed"}, ""},
		{"captured closed, merged", "closed", github.PullRequest{State: "closed", Merged: true}, ""},
		{"schema 1, closed", "", github.PullRequest{State: "closed"}, "closed"},
		{"schema 1, open", "", github.PullRequest{State: "open"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClosedSince(run.Target{PRState: tc.captured}, tc.pr); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// FR-010: the confirmation's state comes from the pull request the gates already read, with no request of its own.
func TestGatesReportTheLiveState(t *testing.T) {
	gh, client := newFake(t)
	in := GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "comment", Draft: readyDraft()}
	open, err := Gates(context.Background(), in)
	if err != nil || open.Closed != "" {
		t.Fatalf("open: %+v, %v", open, err)
	}
	before := len(gh.Requests())
	if _, err := Gates(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	perCall := len(gh.Requests()) - before

	gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, URL: prLink, Title: "Add widgets", State: "closed", Merged: true, Author: "author",
		BaseRef: "main", BaseSHA: "2222222222222222222222222222222222222222", HeadSHA: headSHA})
	before = len(gh.Requests())
	merged, err := Gates(context.Background(), in)
	if err != nil || merged.Closed != "merged" || merged.HeadMoved != nil {
		t.Fatalf("merged: %+v, %v", merged, err)
	}
	if n := len(gh.Requests()) - before; n != perCall {
		t.Fatalf("a merged pull request cost %d requests, an open one %d", n, perCall)
	}
}

// A GitHub outage is not a defect in loupe, so a failed read refuses as github rather than reaching the CLI as internal.
func TestGatesRefuseAFailedGitHubRead(t *testing.T) {
	for _, path := range []string{"/repos/acme/widgets/pulls/42", "/user", "/repos/acme/widgets/pulls/42/commits"} {
		t.Run(path, func(t *testing.T) {
			gh, client := newFake(t)
			if strings.HasSuffix(path, "/commits") {
				gh.SetHead("acme", "widgets", 42, "3333333333333333333333333333333333333333")
			}
			gh.Fail("GET", path, 500)
			in := GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "comment", Draft: readyDraft()}
			gated, err := Gates(context.Background(), in)
			if gated != (Gated{}) {
				t.Errorf("gated %+v", gated)
			}
			wantReadRefusal(t, err, 500)
		})
	}
}

func TestGatesKeepTheClientsOwnRefusal(t *testing.T) {
	gh, client := newFake(t)
	gh.Fail("GET", "/repos/acme/widgets/pulls/42", 401)
	in := GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "comment", Draft: readyDraft()}
	wantRefusal(t, gateErr(context.Background(), in), refusal.Auth, "gh auth login")
}
