package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/secondshift-labs/loupe/internal/draft"
	"github.com/secondshift-labs/loupe/internal/publish"
)

// verdictModel opens review on a ready draft whose f-001 is a blocking issue, gives it verdict, applies change, and
// presses p.
func verdictModel(t *testing.T, viewer, author, verdict string, height int, change func(d *draft.Draft) error) *Model {
	t.Helper()
	dir := readyFixture(t, viewer, author)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		d.Verdict = verdict
		if change != nil {
			return change(d)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "C"}, 100, height)
	m.Update(keyOf("p"))
	if m.view != viewAction {
		t.Fatalf("p did not open the action step: view %v notice %q", m.view, m.notice)
	}
	return m
}

func excludeBlocking(d *draft.Draft) error {
	_, err := draft.Exclude(d, "f-001", testNow)
	return err
}

func wantStart(t *testing.T, m *Model, action string) {
	t.Helper()
	if got := publish.Actions[m.pick]; got != action {
		t.Fatalf("start %q, want %q:\n%s", got, action, m.View().Content)
	}
}

func TestActionStepStartsOnTheVerdict(t *testing.T) {
	for _, c := range []struct {
		verdict, description string
		change               func(d *draft.Draft) error
	}{
		{"approve", "post findings and approve", excludeBlocking},
		{"request-changes", "post findings and request changes", nil},
		{"comment", "post findings, no verdict", nil},
	} {
		t.Run(c.verdict, func(t *testing.T) {
			m := verdictModel(t, "reviewer", "author", c.verdict, 24, c.change)
			wantStart(t, m, c.verdict)
			text := paneText(m.View().Content)
			if !strings.Contains(text, c.verdict+" recommended "+m.glyphs.Sep+" "+c.description) || strings.Count(text, "recommended") != 1 {
				t.Fatalf("the %s row is not the one marked recommended:\n%s", c.verdict, m.View().Content)
			}
		})
	}
}

func TestActionStepWithoutAVerdictIsUnchanged(t *testing.T) {
	m := verdictModel(t, "reviewer", "author", "", 24, nil)
	wantStart(t, m, "comment")
	if strings.Contains(m.View().Content, "recommended") {
		t.Fatalf("a mark without a verdict:\n%s", m.View().Content)
	}
}

// The agent's row says why the cursor is elsewhere, so the step draws no line of its own for it.
func TestActionStepOverridesTheVerdictOnItsRow(t *testing.T) {
	for _, c := range []struct {
		name, viewer, verdict, start, row string
		change                            func(d *draft.Draft) error
	}{
		{"approve with a blocking finding", "reviewer", "approve", "request-changes", "approve recommended %s f-001 is blocking", nil},
		{"request-changes with nothing blocking", "reviewer", "request-changes", "comment", "request-changes recommended %s no published finding is blocking", excludeBlocking},
		{"approve on the human's own pull request", "author", "approve", "comment", "approve recommended %s your own pull request", excludeBlocking},
		{"approve with a blocking finding on the human's own pull request", "author", "approve", "comment", "approve recommended %s your own pull request", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := verdictModel(t, c.viewer, "author", c.verdict, 24, c.change)
			wantStart(t, m, c.start)
			text := paneText(m.View().Content)
			if want := fmt.Sprintf(c.row, m.glyphs.Sep); !strings.Contains(text, want) || strings.Count(text, "recommended") != 1 {
				t.Fatalf("no single %q:\n%s", want, m.View().Content)
			}
			m.Update(keyOf("k"))
			if text := paneText(m.View().Content); !strings.Contains(text, fmt.Sprintf(c.row, m.glyphs.Sep)) {
				t.Fatalf("the reason went with a key:\n%s", m.View().Content)
			}
		})
	}
}

// At 80 columns no row wraps, whatever it says. A narrower popup wraps under the description column.
func TestActionRowsStayOneLineEach(t *testing.T) {
	for _, viewer := range []string{"reviewer", "author"} {
		m := verdictModel(t, viewer, "author", "approve", 24, nil)
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		lines := strings.Split(m.View().Content, "\n")
		first := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, " comment ") })
		if first < 0 || !strings.Contains(lines[first+1], " approve ") || !strings.Contains(lines[first+2], " request-changes ") {
			t.Fatalf("viewer %s: a row wrapped:\n%s", viewer, m.View().Content)
		}
	}
}

func TestEnterOnTheStartIsNeverRefused(t *testing.T) {
	m := verdictModel(t, "author", "author", "approve", 24, nil)
	m.Update(keyOf("enter"))
	if m.view != viewInline || m.action != "comment" {
		t.Fatalf("enter on the start: view %v action %q notice %q", m.view, m.action, m.notice)
	}
}

// The start is worked out when p opens the step, not when the human comes back to it.
func TestBackFromInlineKeepsTheHumansAction(t *testing.T) {
	m := verdictModel(t, "reviewer", "author", "approve", 24, nil)
	wantStart(t, m, "request-changes")
	for _, key := range []string{"k", "k", "enter", "esc"} {
		m.Update(keyOf(key))
	}
	wantStart(t, m, "comment")
	m.Update(keyOf("esc"))
	m.Update(keyOf("p"))
	wantStart(t, m, "request-changes")
}

// However many findings block, the agent's row names a few and counts the rest, so it stays one line.
func TestManyBlockingFindingsStayOnOneRow(t *testing.T) {
	m := verdictModel(t, "reviewer", "author", "approve", 24, func(d *draft.Draft) error {
		for i := 4; i < 64; i++ {
			id := fmt.Sprintf("f-%03d", i)
			d.Findings = append(d.Findings, draft.Finding{ID: id, Rev: 1, Title: "T", Body: "B", General: true, Label: "issue",
				Blocking: true, By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}})
			if _, err := draft.Accept(d, id, "", testNow); err != nil {
				return err
			}
		}
		return nil
	})
	wantStart(t, m, "request-changes")
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	view := m.View().Content
	if !strings.Contains(paneText(view), "f-001, f-004 and 59 more are blocking") || !strings.Contains(view, " request-changes ") {
		t.Fatalf("the row lost its count or the cursor's choice left:\n%s", view)
	}
}
