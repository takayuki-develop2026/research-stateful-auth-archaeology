# PolicyEvaluateResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**input_hash** | **str** |  | 
**policy_evaluation_id** | **int** |  | 
**result** | **str** |  | 
**trace_id** | **str** |  | 

## Example

```python
from openapi_client.models.policy_evaluate_response import PolicyEvaluateResponse

# TODO update the JSON string below
json = "{}"
# create an instance of PolicyEvaluateResponse from a JSON string
policy_evaluate_response_instance = PolicyEvaluateResponse.from_json(json)
# print the JSON string representation of the object
print(PolicyEvaluateResponse.to_json())

# convert the object into a dict
policy_evaluate_response_dict = policy_evaluate_response_instance.to_dict()
# create an instance of PolicyEvaluateResponse from a dict
policy_evaluate_response_from_dict = PolicyEvaluateResponse.from_dict(policy_evaluate_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


