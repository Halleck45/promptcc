<?php

$systemPrompt = <<<PROMPT
You are a billing assistant for {$companyName}.
If the invoice is overdue, escalate to collections.
Unless the customer disputes, always confirm the amount.
Never share internal pricing rules.
PROMPT;

$response = $client->chat()->create([
    'model' => 'gpt-4o',
    'messages' => [
        ['role' => 'system', 'content' => $systemPrompt],
        ['role' => 'user', 'content' => "Summarize the invoice for $customerName briefly."],
    ],
]);

$label = 'ok';
