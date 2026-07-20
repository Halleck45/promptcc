#!/usr/bin/env python3
"""
promptmetrics — analyseur statique de prompts.

MVP. Prend un prompt en texte, sort des métriques qui comptent des choses
distinctes plutôt que du volume. Inspiré de "Rethinking Complexity Metrics
for LLM-Integrated Applications" (2026) : la longueur ne prédit rien, la
ramification (densité de décisions) prédit tout.

Usage:
    python promptmetrics.py prompt.txt
    python promptmetrics.py prompt.txt --json
    cat prompt.txt | python promptmetrics.py -
    python promptmetrics.py p1.txt p2.txt   # comparaison
"""

import argparse
import json
import re
import sys
from dataclasses import dataclass, asdict, field

# --- Lexiques (FR + EN). Volontairement explicites : c'est un artefact à relire. ---

DECISION = [
    # FR
    r"\bsi\b", r"\bsinon\b", r"\bquand\b", r"\blorsque\b", r"\bau cas où\b",
    r"\bsauf si\b", r"\bà moins que\b", r"\bselon\b", r"\bdans le cas\b",
    r"\bsi et seulement si\b", r"\bdès que\b", r"\btant que\b",
    # EN
    r"\bif\b", r"\belse\b", r"\bunless\b", r"\bwhen\b", r"\bwhenever\b",
    r"\bin case\b", r"\bwhether\b", r"\bdepending on\b", r"\bas soon as\b",
    r"\botherwise\b", r"\bshould\b(?=.{0,40}\b(then|,))",
]

CONSTRAINT = [
    # interdictions / obligations fortes — corrélation NÉGATIVE avec la maintenance
    # (plus il y en a, plus le prompt est facile à maintenir)
    r"\bne\s+(?:jamais|pas|plus)\b", r"\bjamais\b", r"\bne\s+\w+\s+jamais\b",
    r"\btu\s+dois\b", r"\bil\s+faut\b", r"\bimpératif\b", r"\bobligatoire\b",
    r"\binterdit\b", r"\bne\s+réponds?\s+(?:pas|jamais)\b",
    r"\bnever\b", r"\balways\b", r"\byou must\b", r"\bmust not\b", r"\bdo not\b",
    r"\bdon'?t\b", r"\bnever\b", r"\bmandatory\b", r"\brequired\b", r"\bforbidden\b",
]

ROLE = [
    r"\btu es\b", r"\bton rôle\b", r"\bagis comme\b", r"\bcomporte-toi\b",
    r"\byou are\b", r"\byour role\b", r"\bact as\b", r"\bbehave as\b",
    r"\bassistant\b", r"\bexpert\b",
]

TOOL_ROUTE = [
    r"\broute\b", r"\brenvoie vers\b", r"\bescalade\b", r"\bappelle?\b.{0,20}\boutil\b",
    r"\butilise l'outil\b", r"\bcall\b.{0,20}\btool\b", r"\bhand off\b",
    r"\bdelegate\b", r"\btransfert\b", r"\bredirige\b",
]

# --- Injection : slots par lesquels du code pousse des valeurs dans le prompt ---

INJECTION_PATTERNS = [
    (r"\{\{\s*\w+\s*\}\}", "handlebars/jinja {{...}}"),
    (r"\{\s*[a-zA-Z_]\w*\s*\}", "str.format / f-string {...}"),
    (r"%\([a-zA-Z_]\w*\)[sd]", "printf-style %(name)s"),
    (r"\$\{?\w+\}?", "shell/PHP $var"),
    (r"<[a-zA-Z_]\w*>", "placeholder <tag>"),
    (r"\[\[\s*\w+\s*\]\]", "double-bracket [[...]]"),
]

# --- Sortie structurée : détecte une exigence de format + profondeur ---

OUTPUT_HINTS = [
    r"\bjson\b", r"\bschéma\b", r"\bschema\b", r"\bformat de sortie\b",
    r"\boutput format\b", r"\brenvoie\b.{0,20}\{", r"\breturn\b.{0,20}\{",
    r"\bau format\b", r"\bréponds? (?:en|au format)\b",
]


def _count_hits(text, patterns):
    total = 0
    per = {}
    for p in patterns:
        n = len(re.findall(p, text, flags=re.IGNORECASE))
        if n:
            per[p] = n
        total += n
    return total, per


def _instruction_units(text):
    """
    Découpe grossière en 'instructions'. Une puce, une ligne non vide, ou une
    phrase terminée par un point. On prend le max des deux découpages pour ne
    pas sous-estimer les prompts en prose.
    """
    lines = [l.strip(" -*•\t") for l in text.splitlines()]
    lines = [l for l in lines if len(l) > 3]
    sentences = re.split(r"(?<=[.!?])\s+", text)
    sentences = [s.strip() for s in sentences if len(s.strip()) > 3]
    return max(len(lines), 1), lines, sentences


def _max_brace_depth(text):
    """Profondeur d'imbrication d'accolades/crochets — proxy de la profondeur de schéma de sortie."""
    depth = 0
    maxd = 0
    for ch in text:
        if ch in "{[":
            depth += 1
            maxd = max(maxd, depth)
        elif ch in "}]":
            depth = max(0, depth - 1)
    return maxd


@dataclass
class Metrics:
    name: str = "prompt"
    # volume (pour info — ne prédit rien, gardé comme témoin)
    n_chars: int = 0
    n_words: int = 0
    n_instructions: int = 0
    # les signaux qui comptent : des choses distinctes
    n_decisions: int = 0            # branches de décision dans le prompt
    p_dec_ratio: float = 0.0        # densité cyclomatique du prompt (le signal fort)
    n_constraints: int = 0          # garde-fous explicites (corr. négative = bon)
    n_roles: int = 0                # définitions de rôle
    n_tool_routes: int = 0          # points de routage / escalade
    inject_surf: int = 0            # canaux d'injection distincts
    n_inject_slots: int = 0         # nombre total de slots
    output_structured: bool = False
    output_depth: int = 0           # profondeur du schéma de sortie
    # score composite
    branching_score: float = 0.0
    detail: dict = field(default_factory=dict)


def analyze(text, name="prompt"):
    m = Metrics(name=name)
    m.n_chars = len(text)
    m.n_words = len(re.findall(r"\w+", text))
    n_instr, lines, _ = _instruction_units(text)
    m.n_instructions = n_instr

    m.n_decisions, dec_detail = _count_hits(text, DECISION)
    m.n_constraints, _ = _count_hits(text, CONSTRAINT)
    m.n_roles, _ = _count_hits(text, ROLE)
    m.n_tool_routes, _ = _count_hits(text, TOOL_ROUTE)

    # P_dec_ratio : fraction d'instructions qui sont conditionnelles
    cond_lines = sum(
        1 for l in lines
        if any(re.search(p, l, re.IGNORECASE) for p in DECISION)
    )
    m.p_dec_ratio = round(cond_lines / n_instr, 3) if n_instr else 0.0

    # inject_surf : nombre de types de canaux distincts, + total de slots
    channels = 0
    slots = 0
    inj_detail = {}
    for pat, label in INJECTION_PATTERNS:
        hits = re.findall(pat, text)
        if hits:
            channels += 1
            slots += len(hits)
            inj_detail[label] = len(hits)
    m.inject_surf = channels
    m.n_inject_slots = slots

    out_total, _ = _count_hits(text, OUTPUT_HINTS)
    m.output_structured = out_total > 0
    m.output_depth = _max_brace_depth(text)

    # Score composite : la ramification, pas le volume.
    # décisions + densité + routage + surface d'injection, atténué par les garde-fous.
    raw = (
        m.n_decisions * 1.0
        + m.p_dec_ratio * 10.0
        + m.n_tool_routes * 1.5
        + m.inject_surf * 1.0
        + m.output_depth * 0.5
    )
    guardrail_relief = min(m.n_constraints * 0.3, raw * 0.4)  # plafonné à -40%
    m.branching_score = round(max(0.0, raw - guardrail_relief), 2)

    m.detail = {
        "decisions_by_pattern": {k: v for k, v in dec_detail.items()},
        "injection_channels": inj_detail,
        "conditional_instruction_lines": cond_lines,
    }
    return m


def _band(score):
    if score < 5:
        return "FAIBLE", "linéaire, peu de ramification"
    if score < 12:
        return "MODÉRÉ", "quelques décisions à garder cohérentes"
    if score < 22:
        return "ÉLEVÉ", "densité de conditions notable — candidat au refactor"
    return "CRITIQUE", "logique métier dense cachée dans le prompt"


def render_text(m: Metrics):
    band, hint = _band(m.branching_score)
    out = []
    out.append(f"── {m.name} " + "─" * max(0, 40 - len(m.name)))
    out.append(f"  Score de ramification : {m.branching_score}  [{band}] — {hint}")
    out.append("")
    out.append("  Ce qui compte (choses distinctes) :")
    out.append(f"    décisions (branches)        n_decisions   = {m.n_decisions}")
    out.append(f"    densité conditionnelle      P_dec_ratio   = {m.p_dec_ratio}")
    out.append(f"    routage / escalade          n_tool_routes = {m.n_tool_routes}")
    out.append(f"    canaux d'injection          inject_surf   = {m.inject_surf}  ({m.n_inject_slots} slots)")
    out.append(f"    profondeur schéma sortie    output_depth  = {m.output_depth}")
    out.append(f"    garde-fous explicites       n_constraints = {m.n_constraints}  (↓ = maintenabilité ↑)")
    out.append(f"    rôles définis               n_roles       = {m.n_roles}")
    out.append("")
    out.append("  Volume (témoin — ne prédit rien) :")
    out.append(f"    {m.n_words} mots · {m.n_chars} car. · {m.n_instructions} instructions")
    if m.detail["decisions_by_pattern"]:
        out.append("")
        out.append("  Détail décisions :")
        for k, v in sorted(m.detail["decisions_by_pattern"].items(), key=lambda x: -x[1]):
            token = k.replace(r"\b", "").replace("\\", "")
            out.append(f"    {v:>3}×  {token}")
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser(description="Analyseur statique de prompts (MVP).")
    ap.add_argument("files", nargs="+", help="fichier(s) texte, ou '-' pour stdin")
    ap.add_argument("--json", action="store_true", help="sortie JSON")
    args = ap.parse_args()

    results = []
    for f in args.files:
        if f == "-":
            text = sys.stdin.read()
            name = "stdin"
        else:
            with open(f, encoding="utf-8") as fh:
                text = fh.read()
            name = f
        results.append(analyze(text, name=name))

    if args.json:
        print(json.dumps([asdict(r) for r in results], ensure_ascii=False, indent=2))
    else:
        for r in results:
            print(render_text(r))
            print()
        if len(results) > 1:
            print("── comparaison " + "─" * 27)
            for r in sorted(results, key=lambda x: -x.branching_score):
                print(f"  {r.branching_score:>6}  {r.name}")


if __name__ == "__main__":
    main()
