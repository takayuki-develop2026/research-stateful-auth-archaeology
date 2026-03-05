# OpenapiClient::ErrorResponseError

## Properties

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **type** | **String** |  |  |
| **message** | **String** |  |  |
| **trace_id** | **String** |  |  |

## Example

```ruby
require 'openapi_client'

instance = OpenapiClient::ErrorResponseError.new(
  type: bad_request,
  message: missing required fields,
  trace_id: trc_v24_sample
)
```

