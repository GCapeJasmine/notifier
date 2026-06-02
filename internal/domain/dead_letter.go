package domain

import (
	"encoding/json"
	"time"
)

type DeadLetter struct {
	ID           int64
	TenantId     string
	WorkflowName string
	Error        string
	Data         json.RawMessage
	RetryCount   int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
