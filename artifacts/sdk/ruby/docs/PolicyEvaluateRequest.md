# OpenapiClient::PolicyEvaluateRequest

## Properties

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **run_id** | **String** |  |  |
| **policy_version_str** | **String** |  |  |
| **pipeline_version** | **String** |  |  |
| **inputs_evidence_asset_id** | **Integer** |  |  |
| **reason_evidence_asset_id** | **Integer** |  |  |
| **obligations_evidence_asset_id** | **Integer** |  |  |

## Example

```ruby
require 'openapi_client'

instance = OpenapiClient::PolicyEvaluateRequest.new(
  run_id: 00000000-0000-0000-0000-000000000000,
  policy_version_str: v23,
  pipeline_version: v23,
  inputs_evidence_asset_id: 68,
  reason_evidence_asset_id: 69,
  obligations_evidence_asset_id: 44
)
```

