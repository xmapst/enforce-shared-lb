package model

type Event struct {
	BindType  string      `json:"bind_type"`
	EventType string      `json:"event_type"`
	Project   string      `json:"project"`
	Data      interface{} `json:"data"`
}
