# DecisionsApi

All URIs are relative to *http://localhost:9023*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**postDecisionApply**](DecisionsApi.md#postdecisionapply) | **POST** /v1/projects/{project_id}/decisions/{decision_id}/apply | Apply a decision (enqueue action) with gate |
| [**postDecisionApprove**](DecisionsApi.md#postdecisionapprove) | **POST** /v1/projects/{project_id}/decisions/{decision_id}/approve | Approve a decision |
| [**postDecisionPropose**](DecisionsApi.md#postdecisionpropose) | **POST** /v1/projects/{project_id}/decisions | Decision propose (create ledger) |



## postDecisionApply

> DecisionApplyResponse postDecisionApply(projectId, decisionId, decisionApplyRequest)

Apply a decision (enqueue action) with gate

v23 gate - if decision is not approved, actions&#x3D;[] and blocked_reason is returned.

### Example

```ts
import {
  Configuration,
  DecisionsApi,
} from '';
import type { PostDecisionApplyRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({ 
    // Configure HTTP bearer authorization: bearerAuth
    accessToken: "YOUR BEARER TOKEN",
  });
  const api = new DecisionsApi(config);

  const body = {
    // string
    projectId: akproj_0000000000000000000,
    // number
    decisionId: 9,
    // DecisionApplyRequest
    decisionApplyRequest: ...,
  } satisfies PostDecisionApplyRequest;

  try {
    const data = await api.postDecisionApply(body);
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
| **decisionId** | `number` |  | [Defaults to `undefined`] |
| **decisionApplyRequest** | [DecisionApplyRequest](DecisionApplyRequest.md) |  | |

### Return type

[**DecisionApplyResponse**](DecisionApplyResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | OK (including blocked cases) |  * X-Trace-Id -  <br>  |
| **400** | Bad Request |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postDecisionApprove

> DecisionApproveResponse postDecisionApprove(projectId, decisionId, decisionApproveRequest)

Approve a decision

### Example

```ts
import {
  Configuration,
  DecisionsApi,
} from '';
import type { PostDecisionApproveRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({ 
    // Configure HTTP bearer authorization: bearerAuth
    accessToken: "YOUR BEARER TOKEN",
  });
  const api = new DecisionsApi(config);

  const body = {
    // string
    projectId: akproj_0000000000000000000,
    // number
    decisionId: 9,
    // DecisionApproveRequest (optional)
    decisionApproveRequest: ...,
  } satisfies PostDecisionApproveRequest;

  try {
    const data = await api.postDecisionApprove(body);
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
| **decisionId** | `number` |  | [Defaults to `undefined`] |
| **decisionApproveRequest** | [DecisionApproveRequest](DecisionApproveRequest.md) |  | [Optional] |

### Return type

[**DecisionApproveResponse**](DecisionApproveResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | OK |  * X-Trace-Id -  <br>  |
| **400** | Bad Request |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postDecisionPropose

> DecisionProposeResponse postDecisionPropose(projectId, decisionProposeRequest)

Decision propose (create ledger)

### Example

```ts
import {
  Configuration,
  DecisionsApi,
} from '';
import type { PostDecisionProposeRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({ 
    // Configure HTTP bearer authorization: bearerAuth
    accessToken: "YOUR BEARER TOKEN",
  });
  const api = new DecisionsApi(config);

  const body = {
    // string
    projectId: akproj_0000000000000000000,
    // DecisionProposeRequest
    decisionProposeRequest: ...,
  } satisfies PostDecisionProposeRequest;

  try {
    const data = await api.postDecisionPropose(body);
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
| **decisionProposeRequest** | [DecisionProposeRequest](DecisionProposeRequest.md) |  | |

### Return type

[**DecisionProposeResponse**](DecisionProposeResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | OK |  * X-Trace-Id -  <br>  |
| **400** | Bad Request |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

