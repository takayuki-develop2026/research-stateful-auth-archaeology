
# LedgerIngestRun


## Properties

Name | Type
------------ | -------------
`id` | string
`projectId` | string
`mode` | string
`sourceEventKey` | string
`fromTs` | Date
`toTs` | Date
`filter` | { [key: string]: any; }
`idempotencyKey` | string
`status` | string
`runId` | string
`traceId` | string
`policyVersionId` | string
`stats` | { [key: string]: any; }
`evidenceRefs` | Array&lt;string&gt;
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { LedgerIngestRun } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "projectId": null,
  "mode": null,
  "sourceEventKey": null,
  "fromTs": null,
  "toTs": null,
  "filter": null,
  "idempotencyKey": null,
  "status": null,
  "runId": null,
  "traceId": null,
  "policyVersionId": null,
  "stats": null,
  "evidenceRefs": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies LedgerIngestRun

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as LedgerIngestRun
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


