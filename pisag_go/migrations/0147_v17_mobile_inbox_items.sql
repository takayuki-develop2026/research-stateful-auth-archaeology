BEGIN;

CREATE TABLE IF NOT EXISTS mobile_inbox_items (
    id                          bigserial PRIMARY KEY,
    public_id                   text NOT NULL UNIQUE,

    project_id                  text NOT NULL,

    inbox_status                text NOT NULL,
    item_kind                   text NOT NULL,

    source_type                 text NOT NULL,
    source_id                   text NOT NULL,

    run_id                      text,
    trace_id                    text NOT NULL,

    actor_user_id               text,
    assigned_user_id            text,

    priority                    text NOT NULL,
    severity                    text NOT NULL,

    title                       text NOT NULL,
    summary                     text,
    action_required             boolean NOT NULL DEFAULT true,
    stepup_required             boolean NOT NULL DEFAULT false,
    comment_required            boolean NOT NULL DEFAULT false,

    available_action_approve    boolean NOT NULL DEFAULT false,
    available_action_reject     boolean NOT NULL DEFAULT false,
    available_action_ack        boolean NOT NULL DEFAULT false,

    terminal_at                 timestamptz,
    terminal_reason_code        text,

    source_occurred_at          timestamptz,
    first_presented_at          timestamptz,
    first_acknowledged_at       timestamptz,
    approved_at                 timestamptz,
    rejected_at                 timestamptz,
    expired_at                  timestamptz,
    canceled_at                 timestamptz,
    superseded_at               timestamptz,

    due_at                      timestamptz,

    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_mobile_inbox_status
        CHECK (inbox_status IN (
            'pending',
            'acknowledged',
            'approved',
            'rejected',
            'expired',
            'canceled',
            'superseded'
        )),

    CONSTRAINT chk_mobile_inbox_item_kind
        CHECK (item_kind IN (
            'approval_request',
            'incident_ack',
            'alert_ack',
            'review_required',
            'manual_decision',
            'operator_attention'
        )),

    CONSTRAINT chk_mobile_inbox_priority
        CHECK (priority IN (
            'low',
            'normal',
            'high',
            'urgent'
        )),

    CONSTRAINT chk_mobile_inbox_severity
        CHECK (severity IN (
            'info',
            'warning',
            'critical'
        )),

    CONSTRAINT chk_mobile_inbox_terminal_at
        CHECK (
            (
                inbox_status IN ('approved', 'rejected', 'expired', 'canceled', 'superseded')
                AND terminal_at IS NOT NULL
            )
            OR
            (
                inbox_status IN ('pending', 'acknowledged')
                AND terminal_at IS NULL
            )
        ),

    CONSTRAINT chk_mobile_inbox_approved_at
        CHECK (
            inbox_status <> 'approved'
            OR approved_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_inbox_rejected_at
        CHECK (
            inbox_status <> 'rejected'
            OR rejected_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_inbox_expired_at
        CHECK (
            inbox_status <> 'expired'
            OR expired_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_inbox_canceled_at
        CHECK (
            inbox_status <> 'canceled'
            OR canceled_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_inbox_superseded_at
        CHECK (
            inbox_status <> 'superseded'
            OR superseded_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_inbox_actionable_terminal
        CHECK (
            inbox_status NOT IN ('approved', 'rejected', 'expired', 'canceled', 'superseded')
            OR action_required = false
        ),

    CONSTRAINT chk_mobile_inbox_action_flags
        CHECK (
            available_action_approve
            OR available_action_reject
            OR available_action_ack
            OR action_required = false
        )
);

COMMENT ON TABLE mobile_inbox_items IS
'v17 mobile operational queue items. not generic push notifications.';

COMMENT ON COLUMN mobile_inbox_items.source_type IS
'origin type in existing SoT / ops systems.';

COMMENT ON COLUMN mobile_inbox_items.source_id IS
'origin identifier in existing SoT / ops systems.';

COMMENT ON COLUMN mobile_inbox_items.stepup_required IS
'whether approve/reject/ack requires step-up on this item.';

COMMENT ON COLUMN mobile_inbox_items.comment_required IS
'reject or high-importance approve may require comment.';

COMMIT;