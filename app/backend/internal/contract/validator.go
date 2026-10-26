// Package contract validates request and response payloads against the same
// versioned JSON Schema document used to generate client types.
package contract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"copypaste.hk/platform/internal/core"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed openapi.json
var document []byte

type Validator struct{ schemas map[string]*jsonschema.Schema }

func New() (*Validator, error) {
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const location = "https://contracts.copypaste.hk/platform.json"
	if err = compiler.AddResource(location, value); err != nil {
		return nil, err
	}
	var names struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err = json.Unmarshal(document, &names); err != nil {
		return nil, err
	}
	v := &Validator{schemas: map[string]*jsonschema.Schema{}}
	for name := range names.Components.Schemas {
		schema, err := compiler.Compile(location + "#/components/schemas/" + name)
		if err != nil {
			return nil, fmt.Errorf("compile contract %s: %w", name, err)
		}
		v.schemas[name] = schema
	}
	return v, nil
}
func (v *Validator) Validate(name string, body []byte) error {
	schema, ok := v.schemas[name]
	if !ok {
		return fmt.Errorf("undefined contract schema %s", name)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return core.Invalid("body", "请求必须是单个有效 JSON 对象")
	}
	if err = schema.Validate(value); err != nil {
		// Validator diagnostics can contain submitted values. Never expose them,
		// since authentication requests and business payloads contain secrets.
		return core.Invalid("body", "请求字段、类型或格式不符合接口契约")
	}
	return nil
}
