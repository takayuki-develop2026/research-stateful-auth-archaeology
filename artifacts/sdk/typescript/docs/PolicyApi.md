# PolicyApi

All URIs are relative to *http://localhost:9023*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**postPolicyEvaluate**](PolicyApi.md#postpolicyevaluate) | **POST** /v1/projects/{project_id}/policy/evaluate | Policy evaluate (MVP) |



## postPolicyEvaluate

> PolicyEvaluateResponse postPolicyEvaluate(projectId, policyEvaluateRequest)

Policy evaluate (MVP)

### Example

```ts
import {
  Configuration,
  PolicyApi,
} from '';
import type { PostPolicyEvaluateRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({ 
    // Configure HTTP bearer authorization: bearerAuth
    accessToken: "YOUR BEARER TOKEN",
  });
  const api = new PolicyApi(config);

  const body = {
    // string
    projectId: akproj_0000000000000000000,
    // PolicyEvaluateRequest
    policyEvaluateRequest: ...,
  } satisfies PostPolicyEvaluateRequest;

  try {
    const data = await api.postPolicyEvaluate(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **projectId** | `string` |  | [Defaults to `undefined`] |
| **policyEvaluateRequest** | [PolicyEvaluateRequest](PolicyEvaluateRequest.md) |  | |

### Return type

[**PolicyEvaluateResponse**](PolicyEvaluateResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | OK |  * X-Trace-Id -  <br>  |
| **400** | Bad Request (validation) |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

