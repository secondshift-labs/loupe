# Contract changes: `specs/038-review-visual-design/contracts/layout.md`

The publish popup rule lives in that file's paragraph that begins "The Action and Inline comments steps MUST be a popup". `docs/tui-design.md` covers only the finding and its header, so the popup rule stays where it is. The implementation MUST add this paragraph after it, citing `specs/059-recommended-verdict`:

> When the draft has a verdict, the Action step marks that action's row: its description opens with `recommended` and the separator glyph, on an available row and on an unavailable one alike. The mark is plain text in the description's tone, so it reads without color. The cursor opens on the start that `publish.StartAction` returns. When that start differs from the verdict, the reason rows sit under the choices and above any refusal, in the secondary tone, wrapped whole to the popup's width. They stay while the step is open and are kept whole like a refusal, so the choices give way first. The Inline comments step draws no mark and no reason. Without a verdict the Action step is unchanged.

Example at 80 columns, with the verdict `approve` and `f-002` accepted as blocking (without color, cursor on `request-changes`):

```text
╭ Publish review ──────────────────────────────────────────────────────╮
│                                                                      │
│  [Action] → Inline comments → Message                                │
│                                                                      │
│  comment          post the findings without a verdict                │
│  approve          recommended · unavailable: cannot approve while    │
│                   publishable findings are blocking: f-002           │
│❯ request-changes  post the findings and request changes              │
│                                                                      │
│  agent recommended approve; f-002 is blocking                        │
╰ nothing sent yet ────────────────────────────────────────────────────╯
```

The frame shows the rows only. Widths, glyphs and the step tabs follow the existing rules, and the golden is the reference.
