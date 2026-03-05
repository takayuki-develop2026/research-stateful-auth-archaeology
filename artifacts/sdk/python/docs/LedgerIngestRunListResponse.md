# LedgerIngestRunListResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**items** | [**List[LedgerIngestRun]**](LedgerIngestRun.md) |  | 

## Example

```python
from openapi_client.models.ledger_ingest_run_list_response import LedgerIngestRunListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of LedgerIngestRunListResponse from a JSON string
ledger_ingest_run_list_response_instance = LedgerIngestRunListResponse.from_json(json)
# print the JSON string representation of the object
print(LedgerIngestRunListResponse.to_json())

# convert the object into a dict
ledger_ingest_run_list_response_dict = ledger_ingest_run_list_response_instance.to_dict()
# create an instance of LedgerIngestRunListResponse from a dict
ledger_ingest_run_list_response_from_dict = LedgerIngestRunListResponse.from_dict(ledger_ingest_run_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


