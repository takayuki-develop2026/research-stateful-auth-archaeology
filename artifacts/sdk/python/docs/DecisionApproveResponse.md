# DecisionApproveResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**decision_id** | **int** |  | 
**status** | **str** |  | 
**trace_id** | **str** |  | 

## Example

```python
from openapi_client.models.decision_approve_response import DecisionApproveResponse

# TODO update the JSON string below
json = "{}"
# create an instance of DecisionApproveResponse from a JSON string
decision_approve_response_instance = DecisionApproveResponse.from_json(json)
# print the JSON string representation of the object
print(DecisionApproveResponse.to_json())

# convert the object into a dict
decision_approve_response_dict = decision_approve_response_instance.to_dict()
# create an instance of DecisionApproveResponse from a dict
decision_approve_response_from_dict = DecisionApproveResponse.from_dict(decision_approve_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


