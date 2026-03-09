package dto

// Response 通用响应
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// SuccessResponse 成功响应
func SuccessResponse(data interface{}) Response {
	return Response{
		Code:    0,
		Message: "success",
		Data:    data,
	}
}

// ErrorResponse 错误响应
func ErrorResponse(message string) Response {
	return Response{
		Code:    1,
		Message: message,
	}
}

// ErrorResponseWithCode 带错误码的错误响应
func ErrorResponseWithCode(code int, message string) Response {
	return Response{
		Code:    code,
		Message: message,
	}
}
