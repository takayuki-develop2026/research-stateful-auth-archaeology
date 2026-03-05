# EvidenceApi

All URIs are relative to *http://localhost:9023*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getEvidenceAssetByRef**](EvidenceApi.md#getevidenceassetbyref) | **GET** /v1/projects/{project_id}/evidence/{evidence_ref} | Get evidence asset by evidence_ref |



## getEvidenceAssetByRef

> EvidenceAsset getEvidenceAssetByRef(projectId, evidenceRef)

Get evidence asset by evidence_ref

Read-only. Returns evidence_assets row by (project_id, evidence_ref).

### Example

```ts
import {
  Configuration,
  EvidenceApi,
} from '';
import type { GetEvidenceAssetByRefRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({ 
    // Configure HTTP bearer authorization: bearerAuth
    accessToken: "YOUR BEARER TOKEN",
  });
  const api = new EvidenceApi(config);

  const body = {
    // string
    projectId: akproj_0000000000000000000,
    // string
    evidenceRef: 3699b593-a3e2-4dd6-8d8b-9d61f099fa04,
  } satisfies GetEvidenceAssetByRefRequest;

  try {
    const data = await api.getEvidenceAssetByRef(body);
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
| **evidenceRef** | `string` |  | [Defaults to `undefined`] |

### Return type

[**EvidenceAsset**](EvidenceAsset.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | OK |  * X-Trace-Id -  <br>  |
| **404** | Not Found |  * X-Trace-Id -  <br>  |
| **400** | Bad Request |  * X-Trace-Id -  <br>  |
| **500** | Error |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

