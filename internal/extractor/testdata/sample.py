import anthropic

client = anthropic.Anthropic()

SYSTEM_PROMPT = f"""You are a support agent for {company}.
If the user asks for a refund, check the purchase date first.
If the purchase is under 30 days old, call the refund tool.
Otherwise, escalate to a human agent.
Never answer off-topic questions."""


def answer(question: str) -> str:
    response = client.messages.create(
        model="claude-sonnet-5",
        system=SYSTEM_PROMPT,
        messages=[
            {
                "role": "user",
                "content": f"Answer this question briefly and cite sources when available: {question}",
            }
        ],
    )
    return response.content[0].text


GREETING = "hi"  # too short, not a prompt

instructions = (
    "When the ticket mentions billing, route it to billing. "
    "Unless resolved, always follow up with the customer."
)
