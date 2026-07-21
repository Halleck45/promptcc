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

// Not a prompt: an embedded PowerShell hook script. Branchy prose to a
// naive heuristic, but it is code, not instructions for a model.
const POST_HOOK_POWERSHELL = `try {
    $rawInput = [Console]::In.ReadToEnd()
    if ($rawInput) {
        $inputData = $rawInput | ConvertFrom-Json -Depth 100
    }
} catch {
    $inputData = $null
}
Invoke-RestMethod -Method Post -Uri $config.webhook_url -Body ($payload | ConvertTo-Json) | Out-Null
@{ cancel = $false } | ConvertTo-Json -Compress
`;

// Not a prompt: component class names, even under a Prompt* binding.
const PromptInputTabLabel = "mb-2 px-3 font-medium text-muted-foreground text-xs";

// Not a prompt: embedded mock server source code used in tests.
const MOCK_MCP_SERVER = `
let buffer = "";
function write(payload) {
  const body = JSON.stringify(payload);
  process.stdout.write("Content-Length: " + body.length + "\r\n\r\n" + body);
}
function handle(message) {
  if (message.method === "initialize") {
    write({ jsonrpc: "2.0", id: message.id, result: { capabilities: { tools: {} } } });
    return;
  }
  if (message.method === "tools/list") {
    write({ jsonrpc: "2.0", id: message.id, result: { tools: [] } });
    return;
  }
}
`;

// Not prompts: expected MODEL OUTPUT asserted in tests, error constructors,
// diagnostics, UI input boxes and SQL handles.
expect(response.text).toBe("The meaning of life is subjective and depends on each individual.");
expect(response.text).toContain("If you refactor the loop, the function gets simpler and easier to test.");
throw new Error("Wrap your component inside PromptInputProvider before using the usePromptInput hook in the tree.");
console.error("Failed to track prompt submission, the telemetry endpoint rejected the payload with a timeout.");
vscode.window.showInputBox({ prompt: "Enter your prompt for generating the notebook cell (press Enter to confirm)" });
db.prepare("UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?");
const insertSpec = "INSERT OR IGNORE INTO cron_event_log (id, run_id, kind, message) VALUES (?, ?, ?, ?)";

// Not a prompt: codegen template written to disk.
const header = `# AUTO-GENERATED FILE, DO NOT EDIT. Regenerate with the build-proto script whenever the protobuf definitions change in any way.`;
