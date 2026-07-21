<?php

// Pipeline step self-documentation: long natural-language prose bound to
// documentation fields (summary, notes). Not prompts. Regression fixture.
class CheckMediaDependenciesStep
{
    public function description(): StepDescription
    {
        return new StepDescription(
            summary: 'Checks that the MEDIA items this item depends on are ready.',
            notes: 'Executed for every non-MEDIA Discovery item. Dispatch is event-driven: maybeDispatchNonMediaDependents triggers this item only when all MEDIA siblings are completed. If a previous MEDIA item failed: failedCritically. If the invariant (all MEDIA siblings completed) is violated: failedCritically.',
        );
    }
}
