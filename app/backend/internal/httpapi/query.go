package httpapi

import (
	"copypaste.hk/platform/internal/core"
	"net/http"
)

func queryBoolean(r *http.Request, name string) (bool, error) {
	values, present := r.URL.Query()[name]
	if !present {
		return false, nil
	}
	if len(values) != 1 || (values[0] != "true" && values[0] != "false") {
		return false, core.Invalid(name, "布尔筛选仅支持 true 或 false")
	}
	return values[0] == "true", nil
}
