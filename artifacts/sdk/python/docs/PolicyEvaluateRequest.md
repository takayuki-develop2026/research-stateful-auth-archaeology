# PolicyEvaluateRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**run_id** | **str** |  | 
**policy_version_str** | **str** |  | 
**pipeline_version** | **str** |  | 
**inputs_evidence_asset_id** | **int** |  | 
**reason_evidence_asset_id** | **int** |  | 
**obligations_evidence_asset_id** | **int** |  | 

## Example

```python
from openapi_client.models.policy_evaluate_request import PolicyEvaluateRequest

# TODO update the JSON string below
json = "{}"
# create an instance of PolicyEvaluateRequest from a JSON string
policy_evaluate_request_instance = PolicyEvaluateRequest.from_json(json)
# print the JSON string representation of the object
print(PolicyEvaluateRequest.to_json())

# convert the object into a dict
policy_evaluate_request_dict = policy_evaluate_request_instance.to_dict()
# create an instance of PolicyEvaluateRequest from a dict
policy_evaluate_request_from_dict = PolicyEvaluateRequest.from_dict(policy_evaluate_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


