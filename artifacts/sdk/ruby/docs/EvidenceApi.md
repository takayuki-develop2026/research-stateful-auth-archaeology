# OpenapiClient::EvidenceApi

All URIs are relative to *http://localhost:9023*

| Method | HTTP request | Description |
| ------ | ------------ | ----------- |
| [**get_evidence_asset_by_ref**](EvidenceApi.md#get_evidence_asset_by_ref) | **GET** /v1/projects/{project_id}/evidence/{evidence_ref} | Get evidence asset by evidence_ref |


## get_evidence_asset_by_ref

> <EvidenceAsset> get_evidence_asset_by_ref(project_id, evidence_ref)

Get evidence asset by evidence_ref

Read-only. Returns evidence_assets row by (project_id, evidence_ref).

### Examples

```ruby
require 'time'
require 'openapi_client'
# setup authorization
OpenapiClient.configure do |config|
  # Configure Bearer authorization (JWT): bearerAuth
  config.access_token = 'YOUR_BEARER_TOKEN'
end

api_instance = OpenapiClient::EvidenceApi.new
project_id = 'akproj_0000000000000000000' # String | 
evidence_ref = '3699b593-a3e2-4dd6-8d8b-9d61f099fa04' # String | 

begin
  # Get evidence asset by evidence_ref
  result = api_instance.get_evidence_asset_by_ref(project_id, evidence_ref)
  p result
rescue OpenapiClient::ApiError => e
  puts "Error when calling EvidenceApi->get_evidence_asset_by_ref: #{e}"
end
```

#### Using the get_evidence_asset_by_ref_with_http_info variant

This returns an Array which contains the response data, status code and headers.

> <Array(<EvidenceAsset>, Integer, Hash)> get_evidence_asset_by_ref_with_http_info(project_id, evidence_ref)

```ruby
begin
  # Get evidence asset by evidence_ref
  data, status_code, headers = api_instance.get_evidence_asset_by_ref_with_http_info(project_id, evidence_ref)
  p status_code # => 2xx
  p headers # => { ... }
  p data # => <EvidenceAsset>
rescue OpenapiClient::ApiError => e
  puts "Error when calling EvidenceApi->get_evidence_asset_by_ref_with_http_info: #{e}"
end
```

### Parameters

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **project_id** | **String** |  |  |
| **evidence_ref** | **String** |  |  |

### Return type

[**EvidenceAsset**](EvidenceAsset.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

