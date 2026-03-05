# OpenapiClient::DecisionProposeRequest

## Properties

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **run_id** | **String** |  |  |
| **policy_evaluation_id** | **Integer** |  |  |
| **subject_type** | **String** |  |  |
| **subject_id** | **String** |  |  |
| **decision_scope** | **String** |  |  |
| **policy_version_str** | **String** |  |  |
| **pipeline_version** | **String** |  |  |
| **input_hash** | **String** |  |  |
| **inputs_evidence_asset_id** | **Integer** |  |  |
| **obligations_evidence_asset_id** | **Integer** |  |  |

## Example

```ruby
require 'openapi_client'

instance = OpenapiClient::DecisionProposeRequest.new(
  run_id: null,
  policy_evaluation_id: null,
  subject_type: null,
  subject_id: null,
  decision_scope: null,
  policy_version_str: null,
  pipeline_version: null,
  input_hash: null,
  inputs_evidence_asset_id: null,
  obligations_evidence_asset_id: null
)
```

