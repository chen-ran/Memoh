ALTER INDEX public.session_runs_single_active
    RENAME TO session_runs_single_active_with_finishing;

CREATE UNIQUE INDEX session_runs_single_active
    ON public.session_runs (team_id, session_id)
    WHERE state IN ('accepted', 'running', 'waiting_decision', 'finishing');
