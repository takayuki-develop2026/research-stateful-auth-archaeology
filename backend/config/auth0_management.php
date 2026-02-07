<?php

return [
    'domain' => env('AUTH0_MANAGEMENT_DOMAIN'),
  'client_id' => env('AUTH0_MANAGEMENT_CLIENT_ID'),
  'client_secret' => env('AUTH0_MANAGEMENT_CLIENT_SECRET'),
  'audience' => env('AUTH0_MANAGEMENT_AUDIENCE'),
    'verify_result_url' => env('AUTH0_VERIFY_RESULT_URL'),
    'verify_ttl_sec' => (int) env('AUTH0_VERIFY_TTL_SEC', 900),
];