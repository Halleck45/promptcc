# promptcc

Cyclomatic complexity, but for LLM prompts, `CLAUDE.md` and skills.

```bash
npx promptcc .                       # scan a repo: code prompts, CLAUDE.md, skills, rules
npx promptcc .claude/skills/x/SKILL.md
npx promptcc --fail-over 22 .        # CI gate
```

This package is a launcher: on first run it downloads the release binary for your platform from [GitHub releases](https://github.com/halleck45/promptcc/releases) and caches it. The package version matches the promptcc release it downloads.

Documentation, HTML report, GitHub Action and Claude Code plugin: [github.com/halleck45/promptcc](https://github.com/halleck45/promptcc).
