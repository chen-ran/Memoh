ALTER INDEX public.idx_session_runs_recovery
    RENAME TO idx_session_runs_recovery_with_finishing;

CREATE INDEX idx_session_runs_recovery
    ON public.session_runs (team_id, live_generation, run_id)
    WHERE state IN ('accepted', 'running', 'waiting_decision', 'finishing');
