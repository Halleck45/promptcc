<?php

// A prompt-builder class: the enclosing class name is evidence that the
// heredocs inside are prompts, even without prompt-like bindings.
class StoryPromptBuilder
{
    private function buildSystemMessage(string $level): string
    {
        return <<<SYSTEM
        You are a story writer for language learners at level {$level}.
        If the chapter is the first one, introduce the characters.
        Unless told otherwise, keep each segment under 80 words.
        Never use vocabulary above the target level.
        SYSTEM;
    }
}

// Error scopes veto their strings, assertion helpers carry no evidence.
class PromptValidationException
{
    private function promptOrMessages(): string
    {
        return 'You must provide either a prompt or a messages array, and never both of them at once.';
    }
}

class PromptAssertions
{
    private function assertPrompt(): string
    {
        return 'The expected prompt was not found in the recorded requests, check the fake provider setup.';
    }
}
