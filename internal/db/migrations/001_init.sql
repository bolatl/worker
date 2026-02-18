-- Migration 001: Initial schema for the jobs table.
-- Enables UUID generation and creates the jobs table with status, attempts, and timestamps.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type TEXT NOT NULL,
    payload JSONB NOT NULL,

    status TEXT NOT NULL CHECK (status IN ('queued','processing','done','failed')),

    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,

    result JSONB NULL,
    last_error TEXT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    processing_started_at TIMESTAMPTZ NULL
    );

-- Index for filtering jobs by status (e.g., reaper query for stuck processing).
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
-- Index for reaper: find jobs stuck in processing beyond timeout.
CREATE INDEX IF NOT EXISTS idx_jobs_processing_started ON jobs(processing_started_at);
