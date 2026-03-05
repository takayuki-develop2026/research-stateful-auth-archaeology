package postgres

import "encoding/json"

// jsonb helper: nil slice/map must not become "null".
// - [] => "[]"
// - {} => "{}"

func marshalJSONBArrayOrEmpty[T any](slice []T) ([]byte, error) {
	if slice == nil {
		return []byte("[]"), nil
	}
	b, err := json.Marshal(slice)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return []byte("[]"), nil
	}
	return b, nil
}

func marshalJSONBObjectOrEmpty(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return []byte("{}"), nil
	}
	return b, nil
}