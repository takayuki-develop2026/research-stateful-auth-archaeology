
# EvidenceAsset


## Properties

Name | Type
------------ | -------------
`id` | number
`projectId` | string
`evidenceRef` | string
`mediaType` | string
`sourceKind` | string
`sourceUri` | string
`contentSha256` | string
`contentLength` | number
`mimeType` | string
`language` | string
`retentionPolicy` | string
`expiresAtUtc` | Date
`status` | string
`createdByType` | string
`createdById` | string
`createdAt` | Date
`updatedAt` | Date

## Example

```typescript
import type { EvidenceAsset } from ''

// TODO: Update the object below with actual values
const example = {
  "id": null,
  "projectId": null,
  "evidenceRef": null,
  "mediaType": null,
  "sourceKind": null,
  "sourceUri": null,
  "contentSha256": null,
  "contentLength": null,
  "mimeType": null,
  "language": null,
  "retentionPolicy": null,
  "expiresAtUtc": null,
  "status": null,
  "createdByType": null,
  "createdById": null,
  "createdAt": null,
  "updatedAt": null,
} satisfies EvidenceAsset

console.log(example)

// Convert the instance to a JSON string
const exampleJSON: string = JSON.stringify(example)
console.log(exampleJSON)

// Parse the JSON string back to an object
const exampleParsed = JSON.parse(exampleJSON) as EvidenceAsset
console.log(exampleParsed)
```

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


