# OpenapiClient::DecisionsApi

All URIs are relative to *http://localhost:9023*

| Method | HTTP request | Description |
| ------ | ------------ | ----------- |
| [**post_decision_apply**](DecisionsApi.md#post_decision_apply) | **POST** /v1/projects/{project_id}/decisions/{decision_id}/apply | Apply a decision (enqueue action) with gate |
| [**post_decision_approve**](DecisionsApi.md#post_decision_approve) | **POST** /v1/projects/{project_id}/decisions/{decision_id}/approve | Approve a decision |
| [**post_decision_propose**](DecisionsApi.md#post_decision_propose) | **POST** /v1/projects/{project_id}/decisions | Decision propose (create ledger) |


## post_decision_apply

> <DecisionApplyResponse> post_decision_apply(project_id, decision_id, decision_apply_request)

Apply a decision (enqueue action) with gate

v23 gate - if decision is not approved, actions=[] and blocked_reason is returned.

### Examples

```ruby
require 'time'
require 'openapi_client'
# setup authorization
OpenapiClient.configure do |config|
  # Configure Bearer authorization (JWT): bearerAuth
  config.access_token = 'YOUR_BEARER_TOKEN'
end

api_instance = OpenapiClient::DecisionsApi.new
project_id = 'akproj_0000000000000000000' # String | 
decision_id = 9 # Integer | 
decision_apply_request = OpenapiClient::DecisionApplyRequest.new({run_id: 'run_id_example', target_evidence_asset_id: 45, plan_evidence_asset_id: 46}) # DecisionApplyRequest | 

begin
  # Apply a decision (enqueue action) with gate
  result = api_instance.post_decision_apply(project_id, decision_id, decision_apply_request)
  p result
rescue OpenapiClient::ApiError => e
  puts "Error when calling DecisionsApi->post_decision_apply: #{e}"
end
```

#### Using the post_decision_apply_with_http_info variant

This returns an Array which contains the response data, status code and headers.

> <Array(<DecisionApplyResponse>, Integer, Hash)> post_decision_apply_with_http_info(project_id, decision_id, decision_apply_request)

```ruby
begin
  # Apply a decision (enqueue action) with gate
  data, status_code, headers = api_instance.post_decision_apply_with_http_info(project_id, decision_id, decision_apply_request)
  p status_code # => 2xx
  p headers # => { ... }
  p data # => <DecisionApplyResponse>
rescue OpenapiClient::ApiError => e
  puts "Error when calling DecisionsApi->post_decision_apply_with_http_info: #{e}"
end
```

### Parameters

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **project_id** | **String** |  |  |
| **decision_id** | **Integer** |  |  |
| **decision_apply_request** | [**DecisionApplyRequest**](DecisionApplyRequest.md) |  |  |

### Return type

[**DecisionApplyResponse**](DecisionApplyResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json


## post_decision_approve

> <DecisionApproveResponse> post_decision_approve(project_id, decision_id, opts)

Approve a decision

### Examples

```ruby
require 'time'
require 'openapi_client'
# setup authorization
OpenapiClient.configure do |config|
  # Configure Bearer authorization (JWT): bearerAuth
  config.access_token = 'YOUR_BEARER_TOKEN'
end

api_instance = OpenapiClient::DecisionsApi.new
project_id = 'akproj_0000000000000000000' # String | 
decision_id = 9 # Integer | 
opts = {
  decision_approve_request: OpenapiClient::DecisionApproveRequest.new # DecisionApproveRequest | 
}

begin
  # Approve a decision
  result = api_instance.post_decision_approve(project_id, decision_id, opts)
  p result
rescue OpenapiClient::ApiError => e
  puts "Error when calling DecisionsApi->post_decision_approve: #{e}"
end
```

#### Using the post_decision_approve_with_http_info variant

This returns an Array which contains the response data, status code and headers.

> <Array(<DecisionApproveResponse>, Integer, Hash)> post_decision_approve_with_http_info(project_id, decision_id, opts)

```ruby
begin
  # Approve a decision
  data, status_code, headers = api_instance.post_decision_approve_with_http_info(project_id, decision_id, opts)
  p status_code # => 2xx
  p headers # => { ... }
  p data # => <DecisionApproveResponse>
rescue OpenapiClient::ApiError => e
  puts "Error when calling DecisionsApi->post_decision_approve_with_http_info: #{e}"
end
```

### Parameters

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **project_id** | **String** |  |  |
| **decision_id** | **Integer** |  |  |
| **decision_approve_request** | [**DecisionApproveRequest**](DecisionApproveRequest.md) |  | [optional] |

### Return type

[**DecisionApproveResponse**](DecisionApproveResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json


## post_decision_propose

> <DecisionProposeResponse> post_decision_propose(project_id, decision_propose_request)

Decision propose (create ledger)

### Examples

```ruby
require 'time'
require 'openapi_client'
# setup authorization
OpenapiClient.configure do |config|
  # Configure Bearer authorization (JWT): bearerAuth
  config.access_token = 'YOUR_BEARER_TOKEN'
end

api_instance = OpenapiClient::DecisionsApi.new
project_id = 'akproj_0000000000000000000' # String | 
decision_propose_request = OpenapiClient::DecisionProposeRequest.new({run_id: 'run_id_example', policy_evaluation_id: 3.56, subject_type: 'subject_type_example', subject_id: 'subject_id_example', decision_scope: 'decision_scope_example', policy_version_str: 'policy_version_str_example', pipeline_version: 'pipeline_version_example', input_hash: 'input_hash_example', inputs_evidence_asset_id: 3.56, obligations_evidence_asset_id: 3.56}) # DecisionProposeRequest | 

begin
  # Decision propose (create ledger)
  result = api_instance.post_decision_propose(project_id, decision_propose_request)
  p result
rescue OpenapiClient::ApiError => e
  puts "Error when calling DecisionsApi->post_decision_propose: #{e}"
end
```

#### Using the post_decision_propose_with_http_info variant

This returns an Array which contains the response data, status code and headers.

> <Array(<DecisionProposeResponse>, Integer, Hash)> post_decision_propose_with_http_info(project_id, decision_propose_request)

```ruby
begin
  # Decision propose (create ledger)
  data, status_code, headers = api_instance.post_decision_propose_with_http_info(project_id, decision_propose_request)
  p status_code # => 2xx
  p headers # => { ... }
  p data # => <DecisionProposeResponse>
rescue OpenapiClient::ApiError => e
  puts "Error when calling DecisionsApi->post_decision_propose_with_http_info: #{e}"
end
```

### Parameters

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **project_id** | **String** |  |  |
| **decision_propose_request** | [**DecisionProposeRequest**](DecisionProposeRequest.md) |  |  |

### Return type

[**DecisionProposeResponse**](DecisionProposeResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: application/json
- **Accept**: application/json

