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

// DDL schema fixtures (common in unit tests) are SQL too.
class ActivityRepositoryTest
{
    public function setUp(): void
    {
        $this->db->exec('
            CREATE TABLE IF NOT EXISTS activity (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                name VARCHAR(255) NOT NULL,
                is_available_for_free TINYINT NOT NULL DEFAULT 0,
                language_code VARCHAR(10),
                disabled_at DATETIME NULL,
                created_at DATETIME,
                updated_at DATETIME
            )
        ');
    }
}
