"""Real-world patterns: SQL, documentation fields and concatenated prompts."""

import argparse

# Not a prompt: SQL passes the naive prose heuristic.
MONITOR_QUERY = """
    SELECT
        SUM(CASE WHEN items.status = %s THEN 1 ELSE 0 END) AS completed,
        SUM(CASE WHEN items.status = %s THEN 1 ELSE 0 END) AS failed,
        COUNT(*) AS total
    FROM generation_plan_items
    WHERE plan_id = %s
"""

# Not a prompt: documentation field in a config mapping, however long.
STEP_CONFIG = {
    "name": "check_media_deps",
    "description": (
        "Executed for every non-media item. Dispatch is event-driven and "
        "triggers this step only when all media siblings are completed. "
        "If a previous media item failed, the step fails critically. When "
        "the invariant is violated, the step also fails critically, so "
        "that the pipeline never persists a partially generated activity."
    ),
}


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    # Not a prompt: CLI help text, bound to a help argument.
    parser.add_argument(
        "--service",
        help="Which provider to call when generating the image, and how to "
        "report failures: the command prints the full error payload and the "
        "duration of every attempt so the failure can be reproduced.",
    )
    return parser


def build_prompt(context: str) -> str:
    # True positive: concatenated prompt, the helper call becomes a slot.
    system_prompt = (
        "You are a grader. If the answer is off-topic, give zero. "
        + get_rubric(context)
        + " Never explain your reasoning. Respond in JSON."
    )
    return system_prompt
