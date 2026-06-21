package state

import "time"

type Status struct {
	StorageOK   bool      `json:"storage_ok"`
	QueueClosed bool      `json:"queue_closed"`
	QueueLen    int       `json:"queue_len"`
	QueueCap    int       `json:"queue_cap"`
	LastError   string    `json:"last_error,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

type Metric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}
