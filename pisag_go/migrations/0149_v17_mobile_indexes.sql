BEGIN;

-- =========================================================
-- mobile_devices
-- =========================================================
CREATE INDEX IF NOT EXISTS idx_mobile_devices_project_actor_status
    ON mobile_devices (project_id, actor_user_id, device_status);

CREATE INDEX IF NOT EXISTS idx_mobile_devices_project_fingerprint
    ON mobile_devices (project_id, device_fingerprint);

CREATE INDEX IF NOT EXISTS idx_mobile_devices_active_only
    ON mobile_devices (project_id, actor_user_id, id)
    WHERE device_status = 'active';

CREATE UNIQUE INDEX IF NOT EXISTS uq_mobile_devices_project_actor_fingerprint_active
    ON mobile_devices (project_id, actor_user_id, device_fingerprint)
    WHERE device_status IN ('pending', 'active');

-- =========================================================
-- mobile_stepup_challenges
-- =========================================================
CREATE INDEX IF NOT EXISTS idx_mobile_stepup_project_actor_status
    ON mobile_stepup_challenges (project_id, actor_user_id, challenge_status);

CREATE INDEX IF NOT EXISTS idx_mobile_stepup_device_status
    ON mobile_stepup_challenges (mobile_device_id, challenge_status);

CREATE INDEX IF NOT EXISTS idx_mobile_stepup_target_inbox
    ON mobile_stepup_challenges (target_inbox_item_id)
    WHERE target_inbox_item_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mobile_stepup_run_action
    ON mobile_stepup_challenges (project_id, run_id, action_kind)
    WHERE run_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mobile_stepup_expiry_active
    ON mobile_stepup_challenges (expires_at)
    WHERE challenge_status IN ('issued', 'verified');

CREATE UNIQUE INDEX IF NOT EXISTS uq_mobile_stepup_one_verified_per_scope
    ON mobile_stepup_challenges (
        project_id,
        actor_user_id,
        mobile_device_id,
        challenge_scope_kind,
        action_kind,
        COALESCE(target_inbox_item_id::text, ''),
        COALESCE(target_source_type, ''),
        COALESCE(target_source_id, ''),
        COALESCE(run_id, '')
    )
    WHERE challenge_status IN ('issued', 'verified');

-- =========================================================
-- mobile_inbox_items
-- =========================================================
CREATE INDEX IF NOT EXISTS idx_mobile_inbox_project_status_priority
    ON mobile_inbox_items (project_id, inbox_status, priority);

CREATE INDEX IF NOT EXISTS idx_mobile_inbox_project_assigned_status
    ON mobile_inbox_items (project_id, assigned_user_id, inbox_status);

CREATE INDEX IF NOT EXISTS idx_mobile_inbox_project_actor_status
    ON mobile_inbox_items (project_id, actor_user_id, inbox_status);

CREATE INDEX IF NOT EXISTS idx_mobile_inbox_source
    ON mobile_inbox_items (project_id, source_type, source_id);

CREATE INDEX IF NOT EXISTS idx_mobile_inbox_run
    ON mobile_inbox_items (project_id, run_id)
    WHERE run_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mobile_inbox_due
    ON mobile_inbox_items (due_at)
    WHERE due_at IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mobile_inbox_source_open
    ON mobile_inbox_items (project_id, source_type, source_id)
    WHERE inbox_status IN ('pending', 'acknowledged');

-- =========================================================
-- mobile_action_receipts
-- =========================================================
CREATE INDEX IF NOT EXISTS idx_mobile_receipts_project_actor_attempted
    ON mobile_action_receipts (project_id, actor_user_id, attempted_at DESC);

CREATE INDEX IF NOT EXISTS idx_mobile_receipts_inbox_attempted
    ON mobile_action_receipts (mobile_inbox_item_id, attempted_at DESC);

CREATE INDEX IF NOT EXISTS idx_mobile_receipts_stepup
    ON mobile_action_receipts (mobile_stepup_challenge_id)
    WHERE mobile_stepup_challenge_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mobile_receipts_run
    ON mobile_action_receipts (project_id, run_id)
    WHERE run_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mobile_receipts_idempotency
    ON mobile_action_receipts (project_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

COMMIT;