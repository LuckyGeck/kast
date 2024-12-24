package main

const (
	// OSP uses _openscreen._udp for discovery
	serviceName = "_openscreen._udp"
	domain      = "local."
	port        = 3333 // Default OSP port
)

// Message types as per OSP spec
type MessageType string

const (
	MsgTypeRequest  MessageType = "request"
	MsgTypeResponse MessageType = "response"
	MsgTypeError    MessageType = "error"
)

// Message represents an OSP message
type Message struct {
	Type      MessageType    `json:"type"`
	RequestID int            `json:"requestId,omitempty"`
	Method    string         `json:"method,omitempty"`
	Params    interface{}    `json:"params,omitempty"`
	Result    interface{}    `json:"result,omitempty"`
	Error     *ErrorResponse `json:"error,omitempty"`
}

// ErrorResponse represents an OSP error
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Presentation represents an active presentation
type Presentation struct {
	URL      string `json:"url"`
	ClientID string `json:"clientId"`
}

// ContentRenderer is an interface for rendering different types of content
type ContentRenderer interface {
	Start(url string) error
	Stop()
	Draw()
}

// windowCommand represents a command to control the window
type windowCommand struct {
	action string
	url    string
	result chan error
}
