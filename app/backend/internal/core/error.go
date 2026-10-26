package core

import (
	"errors"
	"net/http"
)

type Error struct {
	Status    int      `json:"-"`
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Details   []Detail `json:"details,omitempty"`
	Retryable bool     `json:"retryable"`
}
type Detail struct {
	Field       string `json:"field"`
	Code        string `json:"code"`
	SafeMessage string `json:"safe_message"`
}

func (e *Error) Error() string { return e.Code }
func Fail(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}
func Invalid(field, message string) *Error {
	e := Fail(http.StatusUnprocessableEntity, "REQUIRED_BUSINESS_FIELD", message)
	e.Details = []Detail{{Field: field, Code: "INVALID_VALUE", SafeMessage: message}}
	return e
}
func Conflict() *Error {
	return Fail(409, "VERSION_CONFLICT", "资料已更新，请比较最新版本后重试")
}
func Missing() *Error {
	return Fail(404, "RESOURCE_NOT_FOUND", "资料不存在或你没有访问权限")
}
func Forbidden() *Error { return Fail(403, "FORBIDDEN", "没有执行此操作的权限") }
func PublicError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Fail(500, "INTERNAL_ERROR", "服务暂时无法完成请求，请保留请求编号")
}
