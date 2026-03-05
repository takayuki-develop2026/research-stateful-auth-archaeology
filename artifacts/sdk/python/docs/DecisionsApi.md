# openapi_client.DecisionsApi

All URIs are relative to *http://localhost:9023*

Method | HTTP request | Description
------------- | ------------- | -------------
[**post_decision_apply**](DecisionsApi.md#post_decision_apply) | **POST** /v1/projects/{project_id}/decisions/{decision_id}/apply | Apply a decision (enqueue action) with gate
[**post_decision_approve**](DecisionsApi.md#post_decision_approve) | **POST** /v1/projects/{project_id}/decisions/{decision_id}/approve | Approve a decision
[**post_decision_propose**](DecisionsApi.md#post_decision_propose) | **POST** /v1/projects/{project_id}/decisions | Decision propose (create ledger)


# **post_decision_apply**
> DecisionApplyResponse post_decision_apply(project_id, decision_id, decision_apply_request)

Apply a decision (enqueue action) with gate

v23 gate - if decision is not approved, actions=[] and blocked_reason is returned.

### Example

* Bearer (JWT) Authentication (bearerAuth):

```python
import openapi_client
from openapi_client.models.decision_apply_request import DecisionApplyRequest
from openapi_client.models.decision_apply_response import DecisionApplyResponse
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
    api_instance = openapi_client.DecisionsApi(api_client)
    project_id = 'akproj_0000000000000000000' # str | 
    decision_id = 9 # int | 
    decision_apply_request = openapi_client.DecisionApplyRequest() # DecisionApplyRequest | 

    try:
        # Apply a decision (enqueue action) with gate
        api_response = api_instance.post_decision_apply(project_id, decision_id, decision_apply_request)
        print("The response of DecisionsApi->post_decision_apply:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling DecisionsApi->post_decision_apply: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **project_id** | **str**|  | 
 **decision_id** | **int**|  | 
 **decision_apply_request** | [**DecisionApplyRequest**](DecisionApplyRequest.md)|  | 

### Return type

[**DecisionApplyResponse**](DecisionApplyResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | OK (including blocked cases) |  * X-Trace-Id -  <br>  |
**400** | Bad Request |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **post_decision_approve**
> DecisionApproveResponse post_decision_approve(project_id, decision_id, decision_approve_request=decision_approve_request)

Approve a decision

### Example

* Bearer (JWT) Authentication (bearerAuth):

```python
import openapi_client
from openapi_client.models.decision_approve_request import DecisionApproveRequest
from openapi_client.models.decision_approve_response import DecisionApproveResponse
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
    api_instance = openapi_client.DecisionsApi(api_client)
    project_id = 'akproj_0000000000000000000' # str | 
    decision_id = 9 # int | 
    decision_approve_request = openapi_client.DecisionApproveRequest() # DecisionApproveRequest |  (optional)

    try:
        # Approve a decision
        api_response = api_instance.post_decision_approve(project_id, decision_id, decision_approve_request=decision_approve_request)
        print("The response of DecisionsApi->post_decision_approve:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling DecisionsApi->post_decision_approve: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **project_id** | **str**|  | 
 **decision_id** | **int**|  | 
 **decision_approve_request** | [**DecisionApproveRequest**](DecisionApproveRequest.md)|  | [optional] 

### Return type

[**DecisionApproveResponse**](DecisionApproveResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | OK |  * X-Trace-Id -  <br>  |
**400** | Bad Request |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **post_decision_propose**
> DecisionProposeResponse post_decision_propose(project_id, decision_propose_request)

Decision propose (create ledger)

### Example

* Bearer (JWT) Authentication (bearerAuth):

```python
import openapi_client
from openapi_client.models.decision_propose_request import DecisionProposeRequest
from openapi_client.models.decision_propose_response import DecisionProposeResponse
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
    api_instance = openapi_client.DecisionsApi(api_client)
    project_id = 'akproj_0000000000000000000' # str | 
    decision_propose_request = openapi_client.DecisionProposeRequest() # DecisionProposeRequest | 

    try:
        # Decision propose (create ledger)
        api_response = api_instance.post_decision_propose(project_id, decision_propose_request)
        print("The response of DecisionsApi->post_decision_propose:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling DecisionsApi->post_decision_propose: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **project_id** | **str**|  | 
 **decision_propose_request** | [**DecisionProposeRequest**](DecisionProposeRequest.md)|  | 

### Return type

[**DecisionProposeResponse**](DecisionProposeResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | OK |  * X-Trace-Id -  <br>  |
**400** | Bad Request |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

