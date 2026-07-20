import Anthropic from "@anthropic-ai/sdk";

const client = new Anthropic();

const systemPrompt = `You are a triage agent for ${teamName}.
If the ticket mentions billing, route it to the billing tool.
When the user is angry, escalate to a human. Never guess.`;

export async function triage(ticket: string): Promise<string> {
  const response = await client.messages.create({
    model: "claude-sonnet-5",
    system: systemPrompt,
    messages: [{ role: "user", content: `Classify this ticket: ${ticket}` }],
  });
  return response.content[0].text;
}

const shortLabel = "OK";
