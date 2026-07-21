# Contributing to promptcc

Thanks for your interest. Small, focused pull requests are the easiest to review and merge.

## Development

Requires Go 1.24+ and a C compiler (tree-sitter grammars are C code).

```bash
make build   # build the promptcc binary
make test    # gofmt check + go vet + go test -race
make eval    # scan pinned real-world repos, diff against eval/expected/
```

## Evaluation on real code

`make eval` scans pinned commits of three open-source LLM projects (aider, cline, prism) and compares the results against the snapshots in [`eval/expected/`](eval/expected). Any change in extraction or scoring shows up as a diff. Every rule in the extractor was motivated by a real finding on real code and is locked by a regression fixture, so false-positive families are caught before release.

If a mismatch is caused by an intentional change (new rule, adjusted weights), refresh the snapshots with `make eval-update` and commit the updated `eval/expected/` files along with your change.

## Adding a language to the lexicons

This is the ideal first contribution. Lexicons live in `internal/analyzer/lexicon.go` as plain word lists grouped by category:

- `decisionEntries`: conditional branching markers ("if", "unless", "si", ...)
- `constraintEntries`: hard obligations and prohibitions ("never", "you must", ...)
- `roleEntries`: persona definitions ("you are", "act as", ...)
- `toolRouteEntries`: routing and escalation markers ("escalate", "hand off", ...)
- `outputHintEntries`: structured output markers ("json", "output format", ...)

Rules:

- Entries match whole word tokens, case-insensitively, Unicode-aware.
- A `*` in an entry skips up to three arbitrary words: "ne * jamais" matches "ne réponds jamais".
- Prefer precise multi-word phrases over broad single words: overly generic entries create false positives (this is why "assistant" and "expert" are not role markers).
- Add a test in `internal/analyzer/analyzer_test.go` with a realistic prompt in your language, asserting the expected counts.

## Reporting issues

A minimal prompt reproducing the miscount plus the expected numbers is the perfect bug report.
