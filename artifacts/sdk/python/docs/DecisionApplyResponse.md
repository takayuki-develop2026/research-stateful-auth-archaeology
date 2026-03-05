# DecisionApplyResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**trace_id** | **str** |  | 
**decision_id** | **int** |  | 
**decision_status** | **str** |  | [optional] 
**decision_kind** | **str** |  | [optional] 
**blocked_reason** | **str** |  | [optional] 
**actions** | [**List[ActionRef]**](ActionRef.md) |  | 

## Example

```python
from openapi_client.models.decision_apply_response import DecisionApplyResponse

# TODO update the JSON string below
json = "{}"
# create an instance of DecisionApplyResponse from a JSON string
decision_apply_response_instance = DecisionApplyResponse.from_json(json)
# print the JSON string representation of the object
print(DecisionApplyResponse.to_json())

# convert the object into a dict
decision_apply_response_dict = decision_apply_response_instance.to_dict()
# create an instance of DecisionApplyResponse from a dict
decision_apply_response_from_dict = DecisionApplyResponse.from_dict(decision_apply_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


