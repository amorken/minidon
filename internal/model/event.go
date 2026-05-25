package model

type EventType string

const (
	EventTypeStatus      EventType = "status"
	EventTypeStatusEdit  EventType = "status_edit"
	EventTypeStatusDelete EventType = "status_delete"
)

type Event struct {
	Type     EventType `json:"type"`
	Status   *Status   `json:"status,omitempty"`
	StatusID string    `json:"status_id,omitempty"`
}
