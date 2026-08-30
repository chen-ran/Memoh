-- 0144_session_run_finishing_active_index
--
-- Keep this file to one statement. golang-migrate can then execute the
-- concurrent build outside an implicit transaction while the old admission
-- guard remains in place. Deliberately omit IF NOT EXISTS: a failed concurrent
-- build can leave an invalid index behind, and silently accepting that object
-- would let the following swap remove the only valid admission guard.

CREATE UNIQUE INDEX CONCURRENTLY session_runs_single_active_with_finishing
    ON public.session_runs (team_id, session_id)
    WHERE state IN ('accepted', 'running', 'waiting_decision', 'finishing');
