BEGIN;

CREATE TABLE IF NOT EXISTS mobile_devices (
    id                          bigserial PRIMARY KEY,
    public_id                   text NOT NULL UNIQUE,

    project_id                  text NOT NULL,
    actor_user_id               text NOT NULL,

    device_status               text NOT NULL,
    device_label                text,
    platform_type               text NOT NULL,
    app_channel                 text NOT NULL,

    device_fingerprint          text NOT NULL,
    device_key_id               text,
    device_public_key_pem       text,
    attestation_format          text,
    attestation_subject         text,

    registered_at               timestamptz NOT NULL DEFAULT now(),
    activated_at                timestamptz,
    last_seen_at                timestamptz,
    disabled_at                 timestamptz,
    revoked_at                  timestamptz,

    rotated_from_device_id      bigint REFERENCES mobile_devices(id),

    created_run_id              text,
    created_trace_id            text NOT NULL,
    updated_trace_id            text NOT NULL,

    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_mobile_devices_status
        CHECK (device_status IN (
            'pending',
            'active',
            'disabled',
            'revoked',
            'rotated'
        )),

    CONSTRAINT chk_mobile_devices_platform
        CHECK (platform_type IN (
            'ios',
            'android',
            'web_mobile',
            'unknown'
        )),

    CONSTRAINT chk_mobile_devices_app_channel
        CHECK (app_channel IN (
            'pwa',
            'native',
            'browser',
            'unknown'
        )),

    CONSTRAINT chk_mobile_devices_disabled_at
        CHECK (
            device_status <> 'disabled'
            OR disabled_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_devices_revoked_at
        CHECK (
            device_status <> 'revoked'
            OR revoked_at IS NOT NULL
        ),

    CONSTRAINT chk_mobile_devices_rotated_ref
        CHECK (
            device_status <> 'rotated'
            OR rotated_from_device_id IS NOT NULL
        )
);

COMMENT ON TABLE mobile_devices IS
'v17 mobile registered devices. device-bound step-up and action origin tracking.';

COMMENT ON COLUMN mobile_devices.project_id IS
'multi-project boundary; text to align with v18/v19/v21 current contract.';

COMMENT ON COLUMN mobile_devices.actor_user_id IS
'human owner of this device.';

COMMENT ON COLUMN mobile_devices.device_status IS
'pending|active|disabled|revoked|rotated';

COMMENT ON COLUMN mobile_devices.device_fingerprint IS
'server-managed stable device fingerprint, not just ua/ip.';

COMMENT ON COLUMN mobile_devices.device_public_key_pem IS
'future-compatible public key material for signed nonce / webauthn style step-up verification.';

COMMIT;