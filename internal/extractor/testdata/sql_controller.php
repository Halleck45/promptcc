<?php

// SQL passes the naive prose heuristic (CASE WHEN ... THEN ... ELSE reads
// like natural language and is full of decision keywords). Regression
// fixture: none of these strings are prompts.
class PipelineMonitorController
{
    public function steps(): array
    {
        return DB::select("
            SELECT
                CASE
                    WHEN payload LIKE '%GenerateAudioStep%' THEN 'audio'
                    WHEN payload LIKE '%GenerateImageStep%' THEN 'image'
                    WHEN payload LIKE '%GenerateQuestionsStep%' THEN 'questions'
                    WHEN payload LIKE '%TranslateQuestionsStep%' THEN 'translate'
                    WHEN payload LIKE '%GenerateTranscriptStep%' THEN 'transcript'
                    WHEN payload LIKE '%PersistItemStep%' THEN 'persist'
                    ELSE 'other'
                END AS step_type,
                COUNT(*) AS total
            FROM jobs
            GROUP BY step_type
        ");
    }

    public function counters(): array
    {
        return DB::select("
            SELECT
                SUM(CASE WHEN generation_plan_items.status = ? THEN 1 ELSE 0 END) AS completed,
                SUM(CASE WHEN generation_plan_items.status = ? THEN 1 ELSE 0 END) AS running,
                SUM(CASE WHEN generation_plan_items.status = ? THEN 1 ELSE 0 END) AS pending,
                SUM(CASE WHEN generation_plan_items.status = ? THEN 1 ELSE 0 END) AS failed,
                COUNT(*) AS total
            FROM generation_plan_items
            WHERE generation_plan_items.plan_id = ?
        ", [$completed, $running, $pending, $failed, $planId]);
    }
}
