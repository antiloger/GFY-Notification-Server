// Package ntypes
package ntypes

type ResponseT struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	Debug   *string     `json:"debug,omitempty"` // Optional debug information
}

func NewResponse(code int, msg string, data interface{}) ResponseT {
	return ResponseT{
		Code:    code,
		Message: msg,
		Data:    data,
	}
}

func NewErrorResponse(code int, msg string, debug string) ResponseT {
	return ResponseT{
		Code:    code,
		Message: msg,
		Debug:   &debug,
	}
}

func NewDebugResponse(code int, msg string, data interface{}, debug *string) ResponseT {
	return ResponseT{
		Code:    code,
		Message: msg,
		Data:    data,
		Debug:   debug,
	}
}
