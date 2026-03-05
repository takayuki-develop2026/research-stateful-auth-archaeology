
# PolicyEvaluateResponse


## Properties

Name | Type
------------ | -------------
`inputHash` | string
`policyEvaluationId` | number
`result` | string
`traceId` | string

## Example

```typescript
import type { PolicyEvaluateResponse } from ''

// TODO: Update the object below with actual values
const example = {
  "inputHash": f475fdf47ee7a1b7d5a381cc56b4dd5c7ad8f25c0f1ae8f904ae9b1328e312cc,
  "policyEvaluationId": 11,
  "result": allow,
  "traceId": trc_v24_ok,
} satisfies PolicyEvaluateResponse

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as PolicyEvaluateResponse
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


