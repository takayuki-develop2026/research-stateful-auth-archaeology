# OpenapiClient::LedgerApi

All URIs are relative to *http://localhost:9023*

| Method | HTTP request | Description |
| ------ | ------------ | ----------- |
| [**get_ledger_ingest_run**](LedgerApi.md#get_ledger_ingest_run) | **GET** /v1/projects/{project_id}/ledger/ingest-runs/{ingest_run_id} | Get ledger ingest run |
| [**list_ledger_ingest_runs**](LedgerApi.md#list_ledger_ingest_runs) | **GET** /v1/projects/{project_id}/ledger/ingest-runs | List ledger ingest runs |


## get_ledger_ingest_run

> <LedgerIngestRun> get_ledger_ingest_run(project_id, ingest_run_id)

Get ledger ingest run

Read-only. Returns a single ingest run by id (v14.1+).

### Examples

```ruby
require 'time'
require 'openapi_client'
# setup authorization
OpenapiClient.configure do |config|
  # Configure Bearer authorization (JWT): bearerAuth
  config.access_token = 'YOUR_BEARER_TOKEN'
end

api_instance = OpenapiClient::LedgerApi.new
project_id = 'akproj_0000000000000000000' # String | 
ingest_run_id = 'b8233105-68ab-4cdd-8118-b6a59d030355' # String | 

begin
  # Get ledger ingest run
  result = api_instance.get_ledger_ingest_run(project_id, ingest_run_id)
  p result
rescue OpenapiClient::ApiError => e
  puts "Error when calling LedgerApi->get_ledger_ingest_run: #{e}"
end
```

#### Using the get_ledger_ingest_run_with_http_info variant

This returns an Array which contains the response data, status code and headers.

> <Array(<LedgerIngestRun>, Integer, Hash)> get_ledger_ingest_run_with_http_info(project_id, ingest_run_id)

```ruby
begin
  # Get ledger ingest run
  data, status_code, headers = api_instance.get_ledger_ingest_run_with_http_info(project_id, ingest_run_id)
  p status_code # => 2xx
  p headers # => { ... }
  p data # => <LedgerIngestRun>
rescue OpenapiClient::ApiError => e
  puts "Error when calling LedgerApi->get_ledger_ingest_run_with_http_info: #{e}"
end
```

### Parameters

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **project_id** | **String** |  |  |
| **ingest_run_id** | **String** |  |  |

### Return type

[**LedgerIngestRun**](LedgerIngestRun.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json


## list_ledger_ingest_runs

> <LedgerIngestRunListResponse> list_ledger_ingest_runs(project_id, opts)

List ledger ingest runs

Read-only. Returns run-based UTL→Ledger ingest runs (v14.1+).

### Examples

```ruby
require 'time'
require 'openapi_client'
# setup authorization
OpenapiClient.configure do |config|
  # Configure Bearer authorization (JWT): bearerAuth
  config.access_token = 'YOUR_BEARER_TOKEN'
end

api_instance = OpenapiClient::LedgerApi.new
project_id = 'akproj_0000000000000000000' # String | 
opts = {
  status: 'accepted', # String | Filter by ingest run status.
  from: Time.parse('2013-10-20T19:20:30+01:00'), # Time | RFC3339 lower bound for created_at (inclusive).
  to: Time.parse('2013-10-20T19:20:30+01:00'), # Time | RFC3339 upper bound for created_at (exclusive).
  limit: 56 # Integer | Max items.
}

begin
  # List ledger ingest runs
  result = api_instance.list_ledger_ingest_runs(project_id, opts)
  p result
rescue OpenapiClient::ApiError => e
  puts "Error when calling LedgerApi->list_ledger_ingest_runs: #{e}"
end
```

#### Using the list_ledger_ingest_runs_with_http_info variant

This returns an Array which contains the response data, status code and headers.

> <Array(<LedgerIngestRunListResponse>, Integer, Hash)> list_ledger_ingest_runs_with_http_info(project_id, opts)

```ruby
begin
  # List ledger ingest runs
  data, status_code, headers = api_instance.list_ledger_ingest_runs_with_http_info(project_id, opts)
  p status_code # => 2xx
  p headers # => { ... }
  p data # => <LedgerIngestRunListResponse>
rescue OpenapiClient::ApiError => e
  puts "Error when calling LedgerApi->list_ledger_ingest_runs_with_http_info: #{e}"
end
```

### Parameters

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **project_id** | **String** |  |  |
| **status** | **String** | Filter by ingest run status. | [optional] |
| **from** | **Time** | RFC3339 lower bound for created_at (inclusive). | [optional] |
| **to** | **Time** | RFC3339 upper bound for created_at (exclusive). | [optional] |
| **limit** | **Integer** | Max items. | [optional][default to 50] |

### Return type

[**LedgerIngestRunListResponse**](LedgerIngestRunListResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

