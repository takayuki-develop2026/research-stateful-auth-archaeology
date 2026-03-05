# OpenapiClient::EvidenceAsset

## Properties

| Name | Type | Description | Notes |
| ---- | ---- | ----------- | ----- |
| **id** | **Integer** |  |  |
| **project_id** | **String** |  |  |
| **evidence_ref** | **String** |  |  |
| **media_type** | **String** |  |  |
| **source_kind** | **String** |  |  |
| **source_uri** | **String** |  | [optional] |
| **content_sha256** | **String** |  |  |
| **content_length** | **Integer** |  |  |
| **mime_type** | **String** |  |  |
| **language** | **String** |  | [optional] |
| **retention_policy** | **String** |  |  |
| **expires_at_utc** | **Time** |  | [optional] |
| **status** | **String** |  |  |
| **created_by_type** | **String** |  |  |
| **created_by_id** | **String** |  | [optional] |
| **created_at** | **Time** |  |  |
| **updated_at** | **Time** |  |  |

## Example

```ruby
require 'openapi_client'

instance = OpenapiClient::EvidenceAsset.new(
  id: null,
  project_id: null,
  evidence_ref: null,
  media_type: null,
  source_kind: null,
  source_uri: null,
  content_sha256: null,
  content_length: null,
  mime_type: null,
  language: null,
  retention_policy: null,
  expires_at_utc: null,
  status: null,
  created_by_type: null,
  created_by_id: null,
  created_at: null,
  updated_at: null
)
```

