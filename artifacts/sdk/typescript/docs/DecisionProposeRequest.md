
# DecisionProposeRequest


## Properties

Name | Type
------------ | -------------
`runId` | string
`policyEvaluationId` | number
`subjectType` | string
`subjectId` | string
`decisionScope` | string
`policyVersionStr` | string
`pipelineVersion` | string
`inputHash` | string
`inputsEvidenceAssetId` | number
`obligationsEvidenceAssetId` | number

## Example

```typescript
import type { DecisionProposeRequest } from ''

// TODO: Update the object below with actual values
const example = {
  "runId": null,
  "policyEvaluationId": null,
  "subjectType": null,
  "subjectId": null,
  "decisionScope": null,
  "policyVersionStr": null,
  "pipelineVersion": null,
  "inputHash": null,
  "inputsEvidenceAssetId": null,
  "obligationsEvidenceAssetId": null,
} satisfies DecisionProposeRequest

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as DecisionProposeRequest
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


