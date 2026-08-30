-- 0145_session_run_finishing_active_index_swap
-- The concurrently built replacement already protects both old and new active
-- states, so this is only a short catalog-name swap.

DROP INDEX IF EXISTS public.session_runs_single_active;
ALTER INDEX public.session_runs_single_active_with_finishing
    RENAME TO session_runs_single_active;
