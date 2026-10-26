package workflow

import "encoding/json"

// Old immutable snapshots predate schedules. Preserve their original JSON
// projection, including when computing their revision digest after an upgrade.
func (p Snapshot) MarshalJSON() ([]byte, error) {
	type plain Snapshot
	raw, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	if p.Kind == "change" {
		var object map[string]json.RawMessage
		if err = json.Unmarshal(raw, &object); err != nil {
			return nil, err
		}
		for _, key := range []string{"contract_no", "counterparty", "document_no", "employee_reimbursement_amount", "suppliers", "line_values", "purchase_variance"} {
			delete(object, key)
		}
		object["amount"], err = json.Marshal(p.Amount)
		if err != nil {
			return nil, err
		}
		return json.Marshal(object)
	}
	if p.Kind != "contract" {
		var object map[string]json.RawMessage
		if err = json.Unmarshal(raw, &object); err != nil {
			return nil, err
		}
		delete(object, "contract_no")
		delete(object, "counterparty")
		if len(p.LineValues) == 0 {
			object["line_values"] = json.RawMessage(`[]`)
		}
		// Null remains meaningful for an incomplete spend draft.
		object["amount"], err = json.Marshal(p.Amount)
		if err != nil {
			return nil, err
		}
		object["employee_reimbursement_amount"], err = json.Marshal(p.EmployeeReimbursementAmount)
		if err != nil {
			return nil, err
		}
		values := p.Suppliers
		if values == nil {
			object["suppliers"] = json.RawMessage(`[]`)
		} else {
			object["suppliers"], err = json.Marshal(values)
			if err != nil {
				return nil, err
			}
		}
		return json.Marshal(object)
	}
	if p.BusinessPayloadSchemaVersion != 0 || p.ContractContent().PaymentSchedule != nil {
		return raw, nil
	}
	var object map[string]json.RawMessage
	if err = json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	var content map[string]json.RawMessage
	if err = json.Unmarshal(object["content"], &content); err != nil {
		return nil, err
	}
	delete(content, "payment_schedule")
	object["content"], err = json.Marshal(content)
	if err != nil {
		return nil, err
	}
	return json.Marshal(object)
}
