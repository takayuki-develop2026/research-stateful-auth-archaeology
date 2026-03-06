BEGIN;

CREATE TABLE IF NOT EXISTS mobile_stepup_challenges (
    id                          bigserial PRIMARY KEY,
    public_id                   text NOT NULL UNIQUE,

    project_id                  text NOT NULL,
    actor_user_id               text NOT NULL,
    mobile_device_id            bigint NOT NULL REFERENCES mobile_devices(id),

    challenge_status            text NOT NULL,
    stepup_method               text NOT NULL,

    challenge_code_hash         text,
    challenge_nonce             text,

    challenge_scope_kind        text NOT NULL,
    action_kind                 text NOT NULL,

    target_inbox_item_id        bigint,
    target_source_type          text,
    target_source_id            text,
    run_id                      text,
    trace_id                    text NOT NULL,

    issued_at                   timestamptz NOT NULL DEFAULT now(),
    expires_at                  timestamptz NOT NULL,
    verified_at                 timestamptz,
    consumed_at                 timestamptz,
    failed_at                   timestamptz,
    revoked_at                  timestamptz,

    verify_attempt_count        integer NOT NULL DEFAULT 0,
    max_verify_attempts         integer NOT NULL DEFAULT 5,

    last_reason_code            text,
    issued_by_user_id           text,
    consumed_by_user_id         text,

    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_mobile_stepup_status
        CHECK (challenge_status IN (
            'issued',
            'verified',
            'consumed',
            'expired',
            'failed',
            'revoked'
        )),

    CONSTRAINT chk_mobile_stepup_method
        CHECK (stepup_method IN (
            'otp',
            'signed_nonce',
            'webauthn',
            'platform_biometric',
            'unknown'
        )),

    CONSTRAINT chk_mobile_stepup_scope_kind
        CHECK (challenge_scope_kind IN (
            'inbox_item',
            'source_target',
            'run_action'
        )),

    CONSTRAINT chk_mobile_stepup_action_kind
        CHECK (action_kind IN (
            'approve',
            'reject',
            'ack'
        )),

    CONSTRAINT chk_mobile_stepup_expires
        CHECK (expires_at > issued_at),

    CONSTRAINT chk_mobile_stepup_verify_attempts
        CHECK (
            verify_attempt_count >= 0
            AND max_verify_attempts >= 1
            AND verify_attempt_count <= max_verify_attempts
        ),

    CONSTRAINT chk_mobile_stepup_verified_at
        CHECK (
            challenge_status NOT IN ('verified', 'consumed')
            OR verified_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_stepup_consumed_at
        CHECK (
            challenge_status <> 'consumed'
            OR consumed_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_stepup_failed_at
        CHECK (
            challenge_status <> 'failed'
            OR failed_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_stepup_revoked_at
        CHECK (
            challenge_status <> 'revoked'
            OR revoked_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_stepup_actor_consume
        CHECK (
            consumed_by_user_id IS NULL
            OR consumed_by_user_id = actor_user_id
        ),

    CONSTRAINT chk_mobile_stepup_scope_presence
        CHECK (
            (challenge_scope_kind = 'inbox_item' AND target_inbox_item_id IS NOT NULL)
            OR
            (challenge_scope_kind = 'source_target' AND target_source_type IS NOT NULL AND target_source_id IS NOT NULL)
            OR
            (challenge_scope_kind = 'run_action' AND run_id IS NOT NULL)
        )
);

COMMENT ON TABLE mobile_stepup_challenges IS
'v17 step-up challenge facts. one-time, short ttl, device-bound, action-bound.';

COMMENT ON COLUMN mobile_stepup_challenges.challenge_status IS
'issued|verified|consumed|expired|failed|revoked';

COMMENT ON COLUMN mobile_stepup_challenges.action_kind IS
'challenge is action-bound; approve challenge cannot be reused for reject.';

COMMENT ON COLUMN mobile_stepup_challenges.last_reason_code IS
'expired|invalid_code|device_mismatch|actor_mismatch|scope_mismatch|already_consumed|permission_denied|terminal_target etc.';

COMMIT;