<?php

return [
    // venv 内の CLI を絶対パス指定（本番・Docker対応）
    'bin' => base_path('python_batch/atlaskernel/.venv/bin/atlaskernel'),

    // 実行制限（秒）
    'timeout' => 10,

    // ログに出すか
    'log_payload' => true,

    'mode' => env('ATLAS_MODE', 'local'),

    'project_id' => env('ATLAS_PROJECT_ID', 'occore'),

    // docker 内は service 名で
    'endpoint_entity' => env('ATLASKERNEL_ENDPOINT_ENTITY', 'http://python_atlaskernel:8000/v1/analyze/entity'),

    // 旧互換（残してOK）
    'endpoint' => env('ATLASKERNEL_ENDPOINT', 'http://python_atlaskernel:8000/v1/analyze'),

    'assets_path' => env(
        'ATLAS_KERNEL_ASSETS_PATH',
        base_path('python_batch/atlaskernel/src/atlaskernel/assets')
    ),
];
