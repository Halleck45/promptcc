# promptcc

Cyclomatic complexity for prompts.

Static analysis tools assume the behavior of a program lives in its code. In LLM-integrated applications, part of that behavior has moved into prompts: routing decisions, guardrails, business rules. A two-line function carrying a fifty-line prompt shows up green in every linter while its git history screams hotspot.

`promptcc` measures what actually predicts prompt maintenance pain: **branching, not volume**. It counts distinct things (decision points, tool routing, injection surface, output schema depth) and reports a composite branching score, in the spirit of McCabe's cyclomatic complexity.

Background reading (in French): [Quand la complexité du code vit dans le prompt](https://blog.lepine.pro/quand-la-complexite-du-code-vit-dans-le-prompt)

## Install

Download a binary from the [releases page](https://github.com/halleck45/promptcc/releases), or:

```bash
go install github.com/halleck45/promptcc/cmd/promptcc@latest
```

The binary is pure Go, statically linked, no libc dependency.

## Usage

```bash
promptcc prompt.txt              # analyze a prompt file
cat prompt.txt | promptcc        # or pipe it in
promptcc a.txt b.txt             # compare several prompts
promptcc --json prompt.txt       # machine-readable output
promptcc --fail-over 22 p.txt    # CI gate: exit 1 above the threshold
```

Example output:

```
── prompt.txt ──────────────────────────────
  Branching score: 10.89  [MODERATE]  a few decisions to keep consistent

  What matters (distinct things):
    decision points          n_decisions   = 3
    decision density         p_dec_ratio   = 0.429
    routing / escalation     n_tool_routes = 2
    injection channels       inject_surf   = 1  (1 slots)
    output schema depth      output_depth  = 1
    explicit guardrails      n_constraints = 3  (more guardrails, easier maintenance)
    role definitions         n_roles       = 1

  Volume (control group, predicts nothing):
    60 words · 378 chars · 7 instruction units (lines)
```

## Metrics

| Metric | What it counts | Signal |
|---|---|---|
| `n_decisions` | conditional branching points ("if", "unless", "si", "sauf si", ...) | strong positive |
| `p_dec_ratio` | fraction of instruction units that are conditional | strong positive |
| `n_tool_routes` | routing, delegation and escalation points | positive |
| `inject_surf` | distinct channels through which code injects values (`{name}`, `{{name}}`, `$var`, `[[name]]`, `%(name)s`, `<placeholder>`) | positive |
| `output_depth` | nesting depth of the requested output schema | mild positive |
| `n_constraints` | explicit guardrails ("never", "always", "ne ... jamais", ...) | **negative** (explicit limits make prompts easier to maintain) |
| `n_roles` | role and persona definitions | informational |
| `n_words`, `n_chars`, `n_instructions` | volume | none, kept as a control group |

Matching is word-token based and Unicode-aware. English and French are built in; paired XML tags (`<context>...</context>`) are recognized as structure, not injection slots.

### The branching score

```
raw    = 1.0 * n_decisions + 10.0 * p_dec_ratio + 1.5 * n_tool_routes
       + 1.0 * inject_surf + 0.5 * output_depth
relief = min(0.3 * n_constraints, 0.4 * raw)
score  = raw - relief
```

Bands: `LOW` (< 5), `MODERATE` (< 12), `HIGH` (< 22), `CRITICAL` (>= 22).

The weights are an honest v0 heuristic: the relative ordering follows published correlations for LLM-integrated applications (decision density dominates, prompt length predicts nothing, explicit guardrails correlate negatively with maintenance pain), but the absolute values are a judgment call. Calibrating them against a labeled corpus is on the roadmap.

## Roadmap

- [ ] Extract prompts directly from source code (Python, TypeScript, PHP) via tree-sitter, so the tool works as a linter on repositories rather than on isolated text files
- [ ] SARIF output for GitHub code scanning
- [ ] pre-commit hook and GitHub Action
- [ ] Score calibration against a labeled corpus
- [ ] More languages in the lexicons (contributions welcome, see below)

## Contributing

The lexicons in [`internal/analyzer/lexicon.go`](internal/analyzer/lexicon.go) are plain word lists: adding support for your language is the perfect first contribution. Entries match whole word tokens, case-insensitively; `*` skips up to three words ("ne * jamais").

```bash
go test ./...
```

## License

MIT
