package xrayconfig

import "encoding/json"

// Definition describes a base Xray configuration structure that can be rendered
// with runtime client information. Each call to Base should return a fresh copy
// so that callers can safely mutate the returned value without affecting future
// renders.
type Definition interface {
	Base() (map[string]interface{}, error)
}

// JSONDefinition implements Definition by decoding a JSON document. A copy of
// the raw payload is kept to ensure Base can be called repeatedly without
// sharing state between renders.
type JSONDefinition struct {
	Raw []byte
}

// Base returns a deep copy of the JSON document as a map so the generator can
// inject client credentials.
func (d JSONDefinition) Base() (map[string]interface{}, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(d.Raw, &root); err != nil {
		return nil, err
	}
	return root, nil
}
