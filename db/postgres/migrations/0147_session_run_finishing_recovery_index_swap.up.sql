-- 0147_session_run_finishing_recovery_index_swap

DROP INDEX IF EXISTS public.idx_session_runs_recovery;
ALTER INDEX public.idx_session_runs_recovery_with_finishing
    RENAME TO idx_session_runs_recovery;
