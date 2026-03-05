# LedgerApi

All URIs are relative to *http://localhost:9023*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getLedgerIngestRun**](LedgerApi.md#getledgeringestrun) | **GET** /v1/projects/{project_id}/ledger/ingest-runs/{ingest_run_id} | Get ledger ingest run |
| [**listLedgerIngestRuns**](LedgerApi.md#listledgeringestruns) | **GET** /v1/projects/{project_id}/ledger/ingest-runs | List ledger ingest runs |



## getLedgerIngestRun

> LedgerIngestRun getLedgerIngestRun(projectId, ingestRunId)

Get ledger ingest run

Read-only. Returns a single ingest run by id (v14.1+).

### Example

```ts
import {
  Configuration,
  LedgerApi,
} from '';
import type { GetLedgerIngestRunRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({ 
    // Configure HTTP bearer authorization: bearerAuth
    accessToken: "YOUR BEARER TOKEN",
  });
  const api = new LedgerApi(config);

  const body = {
    // string
    projectId: akproj_0000000000000000000,
    // string
    ingestRunId: b8233105-68ab-4cdd-8118-b6a59d030355,
  } satisfies GetLedgerIngestRunRequest;

  try {
    const data = await api.getLedgerIngestRun(body);
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
| **ingestRunId** | `string` |  | [Defaults to `undefined`] |

### Return type

[**LedgerIngestRun**](LedgerIngestRun.md)

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


## listLedgerIngestRuns

> LedgerIngestRunListResponse listLedgerIngestRuns(projectId, status, from, to, limit)

List ledger ingest runs

Read-only. Returns run-based UTL→Ledger ingest runs (v14.1+).

### Example

```ts
import {
  Configuration,
  LedgerApi,
} from '';
import type { ListLedgerIngestRunsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const config = new Configuration({ 
    // Configure HTTP bearer authorization: bearerAuth
    accessToken: "YOUR BEARER TOKEN",
  });
  const api = new LedgerApi(config);

  const body = {
    // string
    projectId: akproj_0000000000000000000,
    // 'accepted' | 'running' | 'succeeded' | 'failed_recorded' | Filter by ingest run status. (optional)
    status: status_example,
    // Date | RFC3339 lower bound for created_at (inclusive). (optional)
    from: 2013-10-20T19:20:30+01:00,
    // Date | RFC3339 upper bound for created_at (exclusive). (optional)
    to: 2013-10-20T19:20:30+01:00,
    // number | Max items. (optional)
    limit: 56,
  } satisfies ListLedgerIngestRunsRequest;

  try {
    const data = await api.listLedgerIngestRuns(body);
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
| **status** | `accepted`, `running`, `succeeded`, `failed_recorded` | Filter by ingest run status. | [Optional] [Defaults to `undefined`] [Enum: accepted, running, succeeded, failed_recorded] |
| **from** | `Date` | RFC3339 lower bound for created_at (inclusive). | [Optional] [Defaults to `undefined`] |
| **to** | `Date` | RFC3339 upper bound for created_at (exclusive). | [Optional] [Defaults to `undefined`] |
| **limit** | `number` | Max items. | [Optional] [Defaults to `50`] |

### Return type

[**LedgerIngestRunListResponse**](LedgerIngestRunListResponse.md)

### Authorization

[bearerAuth](../README.md#bearerAuth)

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | OK |  * X-Trace-Id -  <br>  |
| **400** | Bad Request |  * X-Trace-Id -  <br>  |
| **500** | Error |  * X-Trace-Id -  <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

