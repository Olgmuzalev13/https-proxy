package dto

type InMemoryDB struct {
	RequestAndResponseInDB []RequestAndResponse
}

type RequestAndResponse struct {
	Request  Request
	Response Response
}

type Request struct {
	ID        int
	Method    string
	Path      string
	GetParams map[string]any
	Headers   map[string]string
	Cookie    map[string]any
	Body      string
	Secure int
	FastRerequests string
}

type Response struct {
	ID        int
	Code    int
	Message string
	Headers map[string]string
	Body    string
}

type Rerequested struct {
	Old  RequestAndResponse
	NewResponse Response
}

type Scanned struct {
	Info  RequestAndResponse
	SecurityInfo string
	Safe bool
}