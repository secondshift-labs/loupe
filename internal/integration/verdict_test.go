package integration

import "testing"

// The verdict is the agent's recommendation for the publish step's start. show reports it, a later summary without
// one clears it, and an unattended publication still sends COMMENT.
func TestSummaryVerdictRoundTripsAndNeverSetsTheAction(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("finding.json", fullFinding))
	h.mustOK("summary", "--run", runRef, "--body", "One thing.", "--expect-findings", "1", "--verdict", "approve")
	if v := h.mustOK("show", "--run", runRef)["verdict"]; v != "approve" {
		t.Fatalf("verdict %v, want approve", v)
	}
	h.mustOK("summary", "--run", runRef, "--body", "One thing.", "--expect-findings", "1")
	if v, ok := h.mustOK("show", "--run", runRef)["verdict"]; ok {
		t.Fatalf("verdict %v after a summary without one", v)
	}
	h.mustOK("summary", "--run", runRef, "--body", "One thing.", "--expect-findings", "1", "--verdict", "request-changes")
	h.IsTerminal = false
	if stdout, stderr, exit := h.Run("publish", runRef, "--unattended", "--json"); exit != 0 {
		t.Fatalf("publish exit %d stderr %q stdout %q", exit, stderr, stdout)
	}
	if event := h.lastPost()["event"]; event != "COMMENT" {
		t.Fatalf("event %v, want COMMENT", event)
	}
}
