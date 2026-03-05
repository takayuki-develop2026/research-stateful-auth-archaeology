
# PolicyEvaluateRequest


## Properties

Name | Type
------------ | -------------
`runId` | string
`policyVersionStr` | string
`pipelineVersion` | string
`inputsEvidenceAssetId` | number
`reasonEvidenceAssetId` | number
`obligationsEvidenceAssetId` | number

## Example

```typescript
import type { PolicyEvaluateRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "runId": 00000000-0000-0000-0000-000000000000,
  "policyVersionStr": v23,
  "pipelineVersion": v23,
  "inputsEvidenceAssetId": 68,
  "reasonEvidenceAssetId": 69,
  "obligationsEvidenceAssetId": 44,
} satisfies PolicyEvaluateRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PolicyEvaluateRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


