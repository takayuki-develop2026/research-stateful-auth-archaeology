
# DecisionApplyResponse


## Properties

Name | Type
------------ | -------------
`traceId` | string
`decisionId` | number
`decisionStatus` | string
`decisionKind` | string
`blockedReason` | string
`actions` | [Array&lt;ActionRef&gt;](ActionRef.md)

## Example

```typescript
import type { DecisionApplyResponse } from ''

// TODO: Update the object below with actual values
const example = {
  "traceId": null,
  "decisionId": null,
  "decisionStatus": null,
  "decisionKind": null,
  "blockedReason": null,
  "actions": null,
} satisfies DecisionApplyResponse

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as DecisionApplyResponse
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


