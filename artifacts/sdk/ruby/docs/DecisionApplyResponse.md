# OpenapiClient::DecisionApplyResponse

## Properties

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **trace_id** | **String** |  |  |
| **decision_id** | **Integer** |  |  |
| **decision_status** | **String** |  | [optional] |
| **decision_kind** | **String** |  | [optional] |
| **blocked_reason** | **String** |  | [optional] |
| **actions** | [**Array&lt;ActionRef&gt;**](ActionRef.md) |  |  |

## Example

```ruby
require 'openapi_client'

instance = OpenapiClient::DecisionApplyResponse.new(
  trace_id: null,
  decision_id: null,
  decision_status: null,
  decision_kind: null,
  blocked_reason: null,
  actions: null
)
```

