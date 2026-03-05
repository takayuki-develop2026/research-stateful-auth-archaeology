# EvidenceAsset


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **int** |  | 
**project_id** | **str** |  | 
**evidence_ref** | **UUID** |  | 
**media_type** | **str** |  | 
**source_kind** | **str** |  | 
**source_uri** | **str** |  | [optional] 
**content_sha256** | **str** |  | 
**content_length** | **int** |  | 
**mime_type** | **str** |  | 
**language** | **str** |  | [optional] 
**retention_policy** | **str** |  | 
**expires_at_utc** | **datetime** |  | [optional] 
**status** | **str** |  | 
**created_by_type** | **str** |  | 
**created_by_id** | **str** |  | [optional] 
**created_at** | **datetime** |  | 
**updated_at** | **datetime** |  | 

## Example

```python
from openapi_client.models.evidence_asset import EvidenceAsset

# TODO update the JSON string below
json = "{}"
# create an instance of EvidenceAsset from a JSON string
evidence_asset_instance = EvidenceAsset.from_json(json)
# print the JSON string representation of the object
print(EvidenceAsset.to_json())

# convert the object into a dict
evidence_asset_dict = evidence_asset_instance.to_dict()
# create an instance of EvidenceAsset from a dict
evidence_asset_from_dict = EvidenceAsset.from_dict(evidence_asset_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


