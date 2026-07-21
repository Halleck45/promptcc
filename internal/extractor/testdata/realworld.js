// JavaScript patterns, mirroring the TypeScript fixture plus CommonJS.
const Anthropic = require("@anthropic-ai/sdk");
const client = new Anthropic();

// True positive: template literal bound to a prompt-like name.
const systemPrompt = `You are a triage agent for ${teamName}.
If the ticket mentions billing, route it to the billing tool.
When the user is angry, escalate to a human. Never guess.`;

// True positive: string inside a known SDK call.
async function triage(ticket) {
  const response = await client.messages.create({
    model: "claude-sonnet-5",
    system: systemPrompt,
    messages: [{ role: "user", content: `Classify this ticket: ${ticket}` }],
  });
  return response.content[0].text;
}

// True positive: concatenated prompt, the helper call becomes a slot.
const graderPrompt =
  "You are a grader. If the answer is off-topic, give zero. " +
  getRubric(context) +
  " Never explain your reasoning. Respond in JSON.";

// Not a prompt: SQL in a template literal.
const monitorQuery = `
    SELECT
        SUM(CASE WHEN items.status = ? THEN 1 ELSE 0 END) AS completed,
        COUNT(*) AS total
    FROM generation_plan_items
    WHERE plan_id = ?
`;

// Not a prompt: tool documentation field.
const stepConfig = {
  name: "check_media_deps",
  description:
    "Executed for every non-media item. Dispatch is event-driven and " +
    "triggers this step only when all media siblings are completed. " +
    "If a previous media item failed, the step fails critically. When " +
    "the invariant is violated, the step also fails critically, so that " +
    "the pipeline never persists a partially generated activity.",
};
