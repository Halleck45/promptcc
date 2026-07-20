# promptcc

Cyclomatic complexity for prompts.

Static analysis tools assume the behavior of a program lives in its code. In LLM-integrated applications, part of that behavior has moved into prompts: routing decisions, guardrails, business rules. A two-line function carrying a fifty-line prompt shows up green in every linter while its git history screams hotspot.

`promptcc` measures what actually predicts prompt maintenance pain: branching, not volume. It counts distinct things (decision points, tool routing, injection surface, output schema depth) and reports a composite branching score, in the spirit of McCabe's cyclomatic complexity.

Background (in French): [Quand la complexité du code vit dans le prompt](https://blog.lepine.pro/quand-la-complexite-du-code-vit-dans-le-prompt)

## Status

Work in progress. The original Python proof of concept is kept in [docs/poc.py](docs/poc.py) for reference. The Go implementation lives on the development branches.

## License

MIT
