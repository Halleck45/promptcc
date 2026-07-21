<?php

// A Laravel console command: the artisan signature DSL and the command
// description are not prompts, even when they mention prompts and contain
// long natural-language descriptions. Regression fixture.
class TestImageGeneration extends Command
{
    protected $signature = 'app:test-image-generation
        {prompt=A red apple on a white table : raw prompt sent when --template is not used}
        {--service=openai : openai or nano_banana}
        {--template : use a minimal prompt_template instead of the raw prompt}';

    protected $description = 'Test the image generation call directly over HTTP or Lambda Invoke depending on the binding. When the call fails, the command prints the full error payload, the duration of every attempt, and whatever partial response was received, so that the failure can be reproduced and reported.';
}
