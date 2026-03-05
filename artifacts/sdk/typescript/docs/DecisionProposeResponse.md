
# DecisionProposeResponse


## Properties

Name | Type
------------ | -------------
`decisionId` | number
`decisionKey` | string
`status` | string
`traceId` | string

## Example

```typescript
import type { DecisionProposeResponse } from ''

// TODO: Update the object below with actual values
const example = {
  "decisionId": null,
  "decisionKey": null,
  "status": proposed,
  "traceId": null,
} satisfies DecisionProposeResponse

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as DecisionProposeResponse
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


