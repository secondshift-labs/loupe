# Contract changes: `specs/038-review-visual-design/contracts/layout.md`

The publish popup rule lives in that file's paragraph that begins "The Action and Inline comments steps MUST be a popup". `docs/tui-design.md` covers only the finding and its header, so the popup rule stays where it is. The implementation MUST add this paragraph after it, citing `specs/059-recommended-verdict`:

> When the draft has a verdict, the Action step marks that action's row: its description opens with `recommended` in the good tone and the separator glyph, on an available row and on an unavailable one alike. The cursor opens on the start that `publish.StartAction` returns. When that start differs from the verdict, the agent's row gives the reason after the separator, in the warn tone, in place of its description, and no other line repeats it. Every other unavailable row reads `unavailable` and the separator, then a short reason from `publish.Unavailable`, in place of the whole refusal. Enter on it still shows the whole refusal and its fix. The descriptions are short enough that no row wraps at 80 columns. Without color the words carry the meaning. Without a verdict the Action step has no mark.

Example at 80 columns, with the verdict `approve` and `f-002` accepted as blocking (without color, cursor on `request-changes`). The owner chose it after the first build drew a separate reason line, 2026-11-20:

```text
╭─ Publish review ─────────────────────────────────────────────────────╮
│                                                                      │
│ [Action] > Inline comments > Message                                 │
│                                                                      │
│   comment          post findings, no verdict                         │
│   approve          recommended · f-002 is blocking                   │
│ > request-changes  post findings and request changes                 │
│                                                                      │
╰─ nothing sent yet ───────────────────────────────────────────────────╯
```

The frame shows the rows only. Widths, glyphs and the step tabs follow the existing rules, and the golden is the reference.
