# OpenapiClient::PolicyEvaluateResponse

## Properties

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **input_hash** | **String** |  |  |
| **policy_evaluation_id** | **Integer** |  |  |
| **result** | **String** |  |  |
| **trace_id** | **String** |  |  |

## Example

```ruby
require 'openapi_client'

instance = OpenapiClient::PolicyEvaluateResponse.new(
  input_hash: f475fdf47ee7a1b7d5a381cc56b4dd5c7ad8f25c0f1ae8f904ae9b1328e312cc,
  policy_evaluation_id: 11,
  result: allow,
  trace_id: trc_v24_ok
)
```

