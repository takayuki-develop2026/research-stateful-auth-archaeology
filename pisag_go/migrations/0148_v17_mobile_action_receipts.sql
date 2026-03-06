BEGIN;

CREATE TABLE IF NOT EXISTS mobile_action_receipts (
    id                              bigserial PRIMARY KEY,
    public_id                       text NOT NULL UNIQUE,

    project_id                      text NOT NULL,

    action_kind                     text NOT NULL,
    outcome_status                  text NOT NULL,
    reason_code                     text,

    mobile_inbox_item_id            bigint NOT NULL REFERENCES mobile_inbox_items(id),
    mobile_device_id                bigint NOT NULL REFERENCES mobile_devices(id),
    mobile_stepup_challenge_id      bigint REFERENCES mobile_stepup_challenges(id),

    actor_user_id                   text NOT NULL,
    source_type                     text NOT NULL,
    source_id                       text NOT NULL,

    run_id                          text,
    trace_id                        text NOT NULL,

    idempotency_key                 text,
    comment_text                    text,

    attempted_at                    timestamptz NOT NULL DEFAULT now(),
    completed_at                    timestamptz,

    created_at                      timestamptz NOT NULL DEFAULT now(),
    updated_at                      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_mobile_receipts_action_kind
        CHECK (action_kind IN (
            'approve',
            'reject',
            'ack'
        )),

    CONSTRAINT chk_mobile_receipts_outcome
        CHECK (outcome_status IN (
            'attempted',
            'succeeded',
            'denied',
            'expired',
            'failed',
            'already_applied'
        )),

    CONSTRAINT chk_mobile_receipts_completed_at
        CHECK (
            outcome_status = 'attempted'
            OR completed_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_receipts_stepup_required_for_decision
        CHECK (
            action_kind = 'ack'
            OR mobile_stepup_challenge_id IS NOT NULL
        )
);

COMMENT ON TABLE mobile_action_receipts IS
'v17 action receipts for approve/reject/ack from mobile. includes failures and denials for auditability.';

COMMENT ON COLUMN mobile_action_receipts.outcome_status IS
'attempted|succeeded|denied|expired|failed|already_applied';

COMMENT ON COLUMN mobile_action_receipts.idempotency_key IS
'detect duplicate submits from same actor/device/target/action.';

COMMENT ON COLUMN mobile_action_receipts.reason_code IS
'terminal_target|permission_denied|scope_mismatch|expired|already_consumed|already_applied|validation_error|internal_transient etc.';

COMMIT;