# LedgerIngestRun


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **UUID** |  | 
**project_id** | **str** |  | 
**mode** | **str** |  | 
**source_event_key** | **str** | Present when mode&#x3D;single_event. | [optional] 
**from_ts** | **datetime** |  | [optional] 
**to_ts** | **datetime** |  | [optional] 
**filter** | **Dict[str, object]** |  | 
**idempotency_key** | **str** |  | 
**status** | **str** |  | 
**run_id** | **str** |  | 
**trace_id** | **str** |  | 
**policy_version_id** | **str** |  | 
**stats** | **Dict[str, object]** |  | 
**evidence_refs** | **List[UUID]** |  | 
**created_at** | **datetime** |  | 
**updated_at** | **datetime** |  | 

## Example

```python
from openapi_client.models.ledger_ingest_run import LedgerIngestRun

# TODO update the JSON string below
json = "{}"
# create an instance of LedgerIngestRun from a JSON string
ledger_ingest_run_instance = LedgerIngestRun.from_json(json)
# print the JSON string representation of the object
print(LedgerIngestRun.to_json())

# convert the object into a dict
ledger_ingest_run_dict = ledger_ingest_run_instance.to_dict()
# create an instance of LedgerIngestRun from a dict
ledger_ingest_run_from_dict = LedgerIngestRun.from_dict(ledger_ingest_run_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


