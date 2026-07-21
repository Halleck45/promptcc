// Real-world patterns: SQL, documentation fields and concatenated prompts.

// Not a prompt: SQL in a template literal passes the naive prose heuristic.
const monitorQuery = `
    SELECT
        SUM(CASE WHEN items.status = ? THEN 1 ELSE 0 END) AS completed,
        SUM(CASE WHEN items.status = ? THEN 1 ELSE 0 END) AS failed,
        COUNT(*) AS total
    FROM generation_plan_items
    WHERE plan_id = ?
`;

// Not a prompt: tool documentation field, however long and instruction-like.
const stepConfig = {
  name: "check_media_deps",
  description:
    "Executed for every non-media item. Dispatch is event-driven and " +
    "triggers this step only when all media siblings are completed. " +
    "If a previous media item failed, the step fails critically. When " +
    "the invariant is violated, the step also fails critically, so that " +
    "the pipeline never persists a partially generated activity.",
};

// True positive: concatenated prompt, the helper call becomes a slot.
const systemPrompt =
  "You are a grader. If the answer is off-topic, give zero. " +
  getRubric(context) +
  " Never explain your reasoning. Respond in JSON.";
