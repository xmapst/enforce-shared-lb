package utils

type response struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data,omitempty"`
	Message interface{} `json:"message,omitempty"`
}

func Response(code int, data, msg interface{}) interface{} {
	return &response{code, data, msg}
}
