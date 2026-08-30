-- 0146_session_run_finishing_recovery_index
-- Keep concurrent index creation in a single-statement migration. As with
-- 0144, omit IF NOT EXISTS so an invalid leftover can never be mistaken for a
-- usable replacement by the catalog swap.

CREATE INDEX CONCURRENTLY idx_session_runs_recovery_with_finishing
    ON public.session_runs (team_id, live_generation, run_id)
    WHERE state IN ('accepted', 'running', 'waiting_decision', 'finishing');
