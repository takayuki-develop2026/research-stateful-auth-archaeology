# OpenapiClient::HealthApi

All URIs are relative to *http://localhost:9023*

| Method | HTTP request | Description |
| ------ | ------------ | ----------- |
| [**get_health**](HealthApi.md#get_health) | **GET** /health | Health check |


## get_health

> <HealthResponse> get_health

Health check

### Examples

```ruby
require 'time'
require 'openapi_client'
# setup authorization
OpenapiClient.configure do |config|
  # Configure Bearer authorization (JWT): bearerAuth
  config.access_token = 'YOUR_BEARER_TOKEN'
end

api_instance = OpenapiClient::HealthApi.new

begin
  # Health check
  result = api_instance.get_health
  p result
rescue OpenapiClient::ApiError => e
  puts "Error when calling HealthApi->get_health: #{e}"
end
```

#### Using the get_health_with_http_info variant

This returns an Array which contains the response data, status code and headers.

> <Array(<HealthResponse>, Integer, Hash)> get_health_with_http_info

```ruby
begin
  # Health check
  data, status_code, headers = api_instance.get_health_with_http_info
  p status_code # => 2xx
  p headers # => { ... }
  p data # => <HealthResponse>
rescue OpenapiClient::ApiError => e
  puts "Error when calling HealthApi->get_health_with_http_info: #{e}"
end
```

### Parameters

This endpoint does not need any parameter.

### Return type

[**HealthResponse**](HealthResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

