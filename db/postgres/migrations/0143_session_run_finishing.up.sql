-- 0143_session_run_finishing
--
-- A native terminal event is only a proposal until the fenced durable terminal
-- write lands. Keep that proposal in PostgreSQL so an owner crash between the
-- event and live-state cleanup can be completed by the reaper instead of being
-- misclassified as owner loss.

ALTER TABLE public.session_runs
    ADD COLUMN IF NOT EXISTS proposed_terminal_state TEXT,
    ADD COLUMN IF NOT EXISTS proposed_error_code TEXT,
    ADD COLUMN IF NOT EXISTS proposed_error_message TEXT,
    ADD COLUMN IF NOT EXISTS finish_proposed_at TIMESTAMPTZ;

ALTER TABLE public.session_runs
    DROP CONSTRAINT IF EXISTS session_runs_state_check;

ALTER TABLE public.session_runs
    ADD CONSTRAINT session_runs_state_check CHECK (state IN (
        'accepted', 'running', 'waiting_decision', 'finishing',
        'completed', 'aborted', 'failed', 'lost'
    ));

ALTER TABLE public.session_runs
    DROP CONSTRAINT IF EXISTS session_runs_terminal_proposal_check;

ALTER TABLE public.session_runs
    ADD CONSTRAINT session_runs_terminal_proposal_check CHECK (
        proposed_terminal_state IS NULL
        OR proposed_terminal_state IN ('completed', 'aborted', 'failed')
    );

ALTER TABLE public.session_runs
    DROP CONSTRAINT IF EXISTS session_runs_finishing_proposal_check;

ALTER TABLE public.session_runs
    ADD CONSTRAINT session_runs_finishing_proposal_check CHECK (
        state <> 'finishing'
        OR (proposed_terminal_state IS NOT NULL AND finish_proposed_at IS NOT NULL)
    );
