# OpenapiClient::PolicyApi

All URIs are relative to *http://localhost:9023*

| Method | HTTP request | Description |
| ------ | ------------ | ----------- |
| [**post_policy_evaluate**](PolicyApi.md#post_policy_evaluate) | **POST** /v1/projects/{project_id}/policy/evaluate | Policy evaluate (MVP) |


## post_policy_evaluate

> <PolicyEvaluateResponse> post_policy_evaluate(project_id, policy_evaluate_request)

Policy evaluate (MVP)

### Examples

```ruby
require 'time'
require 'openapi_client'
# setup authorization
OpenapiClient.configure do |config|
  # Configure Bearer authorization (JWT): bearerAuth
  config.access_token = 'YOUR_BEARER_TOKEN'
end

api_instance = OpenapiClient::PolicyApi.new
project_id = 'akproj_0000000000000000000' # String | 
policy_evaluate_request = OpenapiClient::PolicyEvaluateRequest.new({run_id: '00000000-0000-0000-0000-000000000000', policy_version_str: 'v23', pipeline_version: 'v23', inputs_evidence_asset_id: 68, reason_evidence_asset_id: 69, obligations_evidence_asset_id: 44}) # PolicyEvaluateRequest | 

begin
  # Policy evaluate (MVP)
  result = api_instance.post_policy_evaluate(project_id, policy_evaluate_request)
  p result
rescue OpenapiClient::ApiError => e
  puts "Error when calling PolicyApi->post_policy_evaluate: #{e}"
end
```

#### Using the post_policy_evaluate_with_http_info variant

This returns an Array which contains the response data, status code and headers.

> <Array(<PolicyEvaluateResponse>, Integer, Hash)> post_policy_evaluate_with_http_info(project_id, policy_evaluate_request)

```ruby
begin
  # Policy evaluate (MVP)
  data, status_code, headers = api_instance.post_policy_evaluate_with_http_info(project_id, policy_evaluate_request)
  p status_code # => 2xx
  p headers # => { ... }
  p data # => <PolicyEvaluateResponse>
rescue OpenapiClient::ApiError => e
  puts "Error when calling PolicyApi->post_policy_evaluate_with_http_info: #{e}"
end
```

### Parameters

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **project_id** | **String** |  |  |
| **policy_evaluate_request** | [**PolicyEvaluateRequest**](PolicyEvaluateRequest.md) |  |  |

### Return type

[**PolicyEvaluateResponse**](PolicyEvaluateResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

