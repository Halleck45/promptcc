# Contributing to promptcc

Thanks for your interest. Small, focused pull requests are the easiest to review and merge.

## Development

Requires Go 1.24+.

```bash
go build ./cmd/promptcc
go test -race ./...
gofmt -l .        # must print nothing
go vet ./...
```

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
