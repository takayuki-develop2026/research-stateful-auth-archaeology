# OpenapiClient::LedgerIngestRun

## Properties

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **id** | **String** |  |  |
| **project_id** | **String** |  |  |
| **mode** | **String** |  |  |
| **source_event_key** | **String** | Present when mode&#x3D;single_event. | [optional] |
| **from_ts** | **Time** |  | [optional] |
| **to_ts** | **Time** |  | [optional] |
| **filter** | **Hash&lt;String, Object&gt;** |  |  |
| **idempotency_key** | **String** |  |  |
| **status** | **String** |  |  |
| **run_id** | **String** |  |  |
| **trace_id** | **String** |  |  |
| **policy_version_id** | **String** |  |  |
| **stats** | **Hash&lt;String, Object&gt;** |  |  |
| **evidence_refs** | **Array&lt;String&gt;** |  |  |
| **created_at** | **Time** |  |  |
| **updated_at** | **Time** |  |  |

## Example

```ruby
require 'openapi_client'

instance = OpenapiClient::LedgerIngestRun.new(
  id: null,
  project_id: null,
  mode: null,
  source_event_key: null,
  from_ts: null,
  to_ts: null,
  filter: null,
  idempotency_key: null,
  status: null,
  run_id: null,
  trace_id: null,
  policy_version_id: null,
  stats: null,
  evidence_refs: null,
  created_at: null,
  updated_at: null
)
```

