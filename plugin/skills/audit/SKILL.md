---
name: audit
description: Audit CLAUDE.md, AGENTS.md, skills, subagents, commands, Cursor and Copilot rules, and prompts embedded in code for hidden branching logic. Use when the user asks whether their CLAUDE.md or a skill is too complex, why a skill never triggers, how to simplify or split agent instructions, or to score, lint or review prompt files before committing them.
argument-hint: "[path]"
allowed-tools: Bash, Read, Glob
---

# Audit prompt files with promptcc

promptcc is a local, lexical analyzer. It scores the branching a prompt encodes (decision points, routing, injection surface, output schema depth), judges Markdown files by their hottest section, and reports hints the score cannot express. Nothing leaves the machine.

## 1. Make sure the binary is available

Check with `promptcc --version`. If it is missing, get it the cheapest way that works, in this order:

1. `npx promptcc --version` (works wherever Node is installed).
2. `go install github.com/halleck45/promptcc/cmd/promptcc@latest` if Go and a C compiler are present.
3. A release binary: `https://github.com/halleck45/promptcc/releases/latest/download/promptcc_<os>_<arch>` where `<os>` is `linux`, `darwin` or `windows` and `<arch>` is `amd64` or `arm64` (Windows adds `.exe`). Put it somewhere on the PATH and make it executable.

Do not try to build from a clone unless the user asks.

## 2. Run it

The argument is a path; default to the current directory.

```bash
promptcc --json $ARGUMENTS
```

For a single file the user named (`CLAUDE.md`, a `SKILL.md`), the text report is more readable than JSON:

```bash
promptcc $ARGUMENTS
```

The JSON is an array of entries. The fields that matter:

- `kind`: `rules` (loaded every session: CLAUDE.md, AGENTS.md, .cursorrules...), `skill`, `agent`, `command`, `template`, or empty for a prompt found in source code.
- `metrics.branching_score` with its band: LOW < 5, MODERATE < 12, HIGH < 22, CRITICAL from 22.
- `score_basis`: when present, the file was judged by this section, not by its total. Say so.
- `sections[]`: every Markdown heading with its own metrics, in document order.
- `hints[]`: `rule`, `severity` (`warn` or `info`), `message`, `line`.

## 3. Report to the user

Lead with what to fix, not with numbers.

1. **Hints first.** They are the actionable part. Group by file. The rules you will see:
   - `description-without-trigger`: the skill says what it does but not when to use it. The description is the only signal Claude reads to decide whether to load a skill. Propose a rewritten description with a "Use when ..." clause.
   - `missing-description`, `missing-frontmatter`, `missing-name`, `invalid-name`, `frontmatter-invalid`: the file will not load or will not trigger. Propose the exact frontmatter.
   - `body-too-long`: a SKILL.md over 500 lines, or a rules file over 300. Suggest what to move into a `references/` file or a dedicated skill.
   - `broken-reference`: the file points at a script or reference file that does not exist next to it. Either create it or fix the path.
   - `duplicate-rule`: the same instruction line appears in several rules files. Keep one.
2. **Then the hot spots.** For each HIGH or CRITICAL entry, name the file, the judging section and its line, and give the remedy that fits the kind: a rules file should hand its branching to a skill that loads only when relevant; a skill should split into a decision table or one skill per case; a prompt in code should route first, then use one specialized prompt per branch. The text report prints this advice under each hotspot.
3. **Numbers last, and briefly.** One line: how many prompts, how many prompt files, the worst score. Do not paste the whole JSON.

If everything is LOW and there are no hints, say so in one sentence and stop.

## 4. Offer to fix

When the user agrees, edit the files directly: rewrite a skill description, split a hot section into a new skill under `.claude/skills/<name>/SKILL.md`, create a missing reference file, or delete a duplicated rule. Re-run promptcc on the edited file and show the before/after score.

Keep edits minimal. Never rewrite a whole CLAUDE.md to lower a score: the score is a signal for a human decision, not a target.
