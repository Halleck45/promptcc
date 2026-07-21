<?php

// A Laravel FormRequest: none of these strings are prompts, even though the
// keys contain the word "prompt". Regression fixture for false positives.
class GenerateImageRequest extends FormRequest
{
    public function rules(): array
    {
        return [
            'prompt' => 'required_without:prompt_template|string|max:4000',
            'prompt_template' => 'required_without:prompt|string|max:255',
        ];
    }

    public function messages(): array
    {
        return [
            'prompt.required_without' => 'Please provide a prompt or a prompt_template.',
            'prompt_template.required_without' => 'Please provide a prompt or a prompt_template.',
        ];
    }
}
