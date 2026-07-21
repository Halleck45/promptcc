<?php

// Patterns observed in real codebases. The one true prompt in this file is
// the concatenated system message pushed into $prompts[]; everything else
// must be ignored.
class AssessmentExecutor
{
    public function execute(array $prompts): array
    {
        $example = json_decode(file_get_contents(__DIR__ . '/example.json'));

        // True positive: concatenation merged into one prompt, json_encode
        // becomes an injection slot.
        $prompts[] = [
            'role' => 'system',
            'content' => 'Respond with JSON only, for example: ' . json_encode($example, JSON_PRETTY_PRINT),
        ];

        // Not prompts: exception messages built by concatenation.
        if (json_last_error() !== JSON_ERROR_NONE) {
            throw new RuntimeException('Failed to decode JSON schema: ' . json_last_error_msg());
        }

        return $this->client->chat()->create([
            'model' => 'gpt-4o',
            'messages' => $prompts,
        ]);
    }
}
