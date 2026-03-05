# DecisionProposeRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**run_id** | **str** |  | 
**policy_evaluation_id** | **int** |  | 
**subject_type** | **str** |  | 
**subject_id** | **str** |  | 
**decision_scope** | **str** |  | 
**policy_version_str** | **str** |  | 
**pipeline_version** | **str** |  | 
**input_hash** | **str** |  | 
**inputs_evidence_asset_id** | **int** |  | 
**obligations_evidence_asset_id** | **int** |  | 

## Example

```python
from openapi_client.models.decision_propose_request import DecisionProposeRequest

# TODO update the JSON string below
json = "{}"
# create an instance of DecisionProposeRequest from a JSON string
decision_propose_request_instance = DecisionProposeRequest.from_json(json)
# print the JSON string representation of the object
print(DecisionProposeRequest.to_json())

# convert the object into a dict
decision_propose_request_dict = decision_propose_request_instance.to_dict()
# create an instance of DecisionProposeRequest from a dict
decision_propose_request_from_dict = DecisionProposeRequest.from_dict(decision_propose_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


