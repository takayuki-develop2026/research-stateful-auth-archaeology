# DecisionApplyRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**run_id** | **str** |  | 
**action_type** | **str** |  | [optional] 
**action_scope** | **str** |  | [optional] 
**target_evidence_asset_id** | **int** |  | 
**plan_evidence_asset_id** | **int** |  | 
**budget_currency** | **str** |  | [optional] 
**budget_estimate_amount** | **int** |  | [optional] 

## Example

```python
from openapi_client.models.decision_apply_request import DecisionApplyRequest

# TODO update the JSON string below
json = "{}"
# create an instance of DecisionApplyRequest from a JSON string
decision_apply_request_instance = DecisionApplyRequest.from_json(json)
# print the JSON string representation of the object
print(DecisionApplyRequest.to_json())

# convert the object into a dict
decision_apply_request_dict = decision_apply_request_instance.to_dict()
# create an instance of DecisionApplyRequest from a dict
decision_apply_request_from_dict = DecisionApplyRequest.from_dict(decision_apply_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


