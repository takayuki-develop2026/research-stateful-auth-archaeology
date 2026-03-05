
# ErrorResponseError


## Properties

Name | Type
------------ | -------------
`type` | string
`message` | string
`traceId` | string

## Example

```typescript
import type { ErrorResponseError } from ''

// TODO: Update the object below with actual values
const example = {
  "type": bad_request,
  "message": missing required fields,
  "traceId": trc_v24_sample,
} satisfies ErrorResponseError

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as ErrorResponseError
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


