# OpenapiClient::DecisionApplyRequest

## Properties

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **run_id** | **String** |  |  |
| **action_type** | **String** |  | [optional] |
| **action_scope** | **String** |  | [optional] |
| **target_evidence_asset_id** | **Integer** |  |  |
| **plan_evidence_asset_id** | **Integer** |  |  |
| **budget_currency** | **String** |  | [optional] |
| **budget_estimate_amount** | **Integer** |  | [optional] |

## Example

```ruby
require 'openapi_client'

instance = OpenapiClient::DecisionApplyRequest.new(
  run_id: null,
  action_type: publish_http,
  action_scope: managed,
  target_evidence_asset_id: 45,
  plan_evidence_asset_id: 46,
  budget_currency: usd_micros,
  budget_estimate_amount: 1000
)
```

