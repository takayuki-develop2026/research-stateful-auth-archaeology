# openapi_client.LedgerApi

All URIs are relative to *http://localhost:9023*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_ledger_ingest_run**](LedgerApi.md#get_ledger_ingest_run) | **GET** /v1/projects/{project_id}/ledger/ingest-runs/{ingest_run_id} | Get ledger ingest run
[**list_ledger_ingest_runs**](LedgerApi.md#list_ledger_ingest_runs) | **GET** /v1/projects/{project_id}/ledger/ingest-runs | List ledger ingest runs


# **get_ledger_ingest_run**
> LedgerIngestRun get_ledger_ingest_run(project_id, ingest_run_id)

Get ledger ingest run

Read-only. Returns a single ingest run by id (v14.1+).

### Example

* Bearer (JWT) Authentication (bearerAuth):

```python
import openapi_client
from openapi_client.models.ledger_ingest_run import LedgerIngestRun
from openapi_client.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:9023
# See configuration.py for a list of all supported configuration parameters.
configuration = openapi_client.Configuration(
    host = "http://localhost:9023"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure Bearer authorization (JWT): bearerAuth
configuration = openapi_client.Configuration(
    access_token = os.environ["BEARER_TOKEN"]
)

# Enter a context with an instance of the API client
with openapi_client.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = openapi_client.LedgerApi(api_client)
    project_id = 'akproj_0000000000000000000' # str | 
    ingest_run_id = UUID('b8233105-68ab-4cdd-8118-b6a59d030355') # UUID | 

    try:
        # Get ledger ingest run
        api_response = api_instance.get_ledger_ingest_run(project_id, ingest_run_id)
        print("The response of LedgerApi->get_ledger_ingest_run:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling LedgerApi->get_ledger_ingest_run: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **project_id** | **str**|  | 
 **ingest_run_id** | **UUID**|  | 

### Return type

[**LedgerIngestRun**](LedgerIngestRun.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | OK |  * X-Trace-Id -  <br>  |
**404** | Not Found |  * X-Trace-Id -  <br>  |
**400** | Bad Request |  * X-Trace-Id -  <br>  |
**500** | Error |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_ledger_ingest_runs**
> LedgerIngestRunListResponse list_ledger_ingest_runs(project_id, status=status, var_from=var_from, to=to, limit=limit)

List ledger ingest runs

Read-only. Returns run-based UTL→Ledger ingest runs (v14.1+).

### Example

* Bearer (JWT) Authentication (bearerAuth):

```python
import openapi_client
from openapi_client.models.ledger_ingest_run_list_response import LedgerIngestRunListResponse
from openapi_client.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:9023
# See configuration.py for a list of all supported configuration parameters.
configuration = openapi_client.Configuration(
    host = "http://localhost:9023"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure Bearer authorization (JWT): bearerAuth
configuration = openapi_client.Configuration(
    access_token = os.environ["BEARER_TOKEN"]
)

# Enter a context with an instance of the API client
with openapi_client.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = openapi_client.LedgerApi(api_client)
    project_id = 'akproj_0000000000000000000' # str | 
    status = 'status_example' # str | Filter by ingest run status. (optional)
    var_from = '2013-10-20T19:20:30+01:00' # datetime | RFC3339 lower bound for created_at (inclusive). (optional)
    to = '2013-10-20T19:20:30+01:00' # datetime | RFC3339 upper bound for created_at (exclusive). (optional)
    limit = 50 # int | Max items. (optional) (default to 50)

    try:
        # List ledger ingest runs
        api_response = api_instance.list_ledger_ingest_runs(project_id, status=status, var_from=var_from, to=to, limit=limit)
        print("The response of LedgerApi->list_ledger_ingest_runs:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling LedgerApi->list_ledger_ingest_runs: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **project_id** | **str**|  | 
 **status** | **str**| Filter by ingest run status. | [optional] 
 **var_from** | **datetime**| RFC3339 lower bound for created_at (inclusive). | [optional] 
 **to** | **datetime**| RFC3339 upper bound for created_at (exclusive). | [optional] 
 **limit** | **int**| Max items. | [optional] [default to 50]

### Return type

[**LedgerIngestRunListResponse**](LedgerIngestRunListResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | OK |  * X-Trace-Id -  <br>  |
**400** | Bad Request |  * X-Trace-Id -  <br>  |
**500** | Error |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

