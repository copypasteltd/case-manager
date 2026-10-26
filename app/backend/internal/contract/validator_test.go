package contract

import "testing"

func TestStrictVersionedSchemas(t *testing.T) {
	validator, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		schema, body string
		valid        bool
	}{
		{"Transition", `{"expected_version":1,"action":"activate","reason":"ready"}`, true},
		{"Transition", `{"expected_version":1,"action":"activate"}`, false},
		{"Transition", `{"expected_version":"1","action":"activate","reason":"ready"}`, false},
		{"Transition", `{"expected_version":1,"action":"activate","reason":"ready","actor_id":"other"}`, false},
		{"ProjectCreate", `{"name":"P","scope":"","owner_id":"bad","classification":"internal"}`, false},
		{"ProjectCreate", `{"name":"P","scope":"","owner_id":"00000000-0000-4000-8000-000000000001","classification":"internal","due_date":"2026-02-30"}`, false},
		{"TaskPatch", `{"expected_version":1,"assignee_id":null}`, true},
		{"TaskPatch", `{"expected_version":1,"title":null}`, false},
		{"EmptyInput", `{} {}`, false},
	} {
		if err = validator.Validate(test.schema, []byte(test.body)); (err == nil) != test.valid {
			t.Errorf("unexpected validation result for %s %s", test.schema, test.body)
		}
	}
}
