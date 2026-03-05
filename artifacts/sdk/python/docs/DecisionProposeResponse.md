# DecisionProposeResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**decision_id** | **int** |  | 
**decision_key** | **str** |  | 
**status** | **str** |  | 
**trace_id** | **str** |  | 

## Example

```python
from openapi_client.models.decision_propose_response import DecisionProposeResponse

# TODO update the JSON string below
json = "{}"
# create an instance of DecisionProposeResponse from a JSON string
decision_propose_response_instance = DecisionProposeResponse.from_json(json)
# print the JSON string representation of the object
print(DecisionProposeResponse.to_json())

# convert the object into a dict
decision_propose_response_dict = decision_propose_response_instance.to_dict()
# create an instance of DecisionProposeResponse from a dict
decision_propose_response_from_dict = DecisionProposeResponse.from_dict(decision_propose_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


