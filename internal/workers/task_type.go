package workers

import "time"

type TaskParams struct {
	ID                      string
	TaskName                string
	ScheduleIntervalSeconds int64
	NextRunAt               time.Time
	UpdatedAt               time.Time
}

type ClaimTaskParams struct {
	LeaseOwner      string
	LeaseExpiresAt  time.Time
	LastRunAt       time.Time
	UpdatedAt       time.Time
	TaskName        string
	LeaseExpiresAt2 time.Time
	NextRunAt       time.Time
}

type FinishTaskParams struct {
	LastFinishedAt   time.Time
	NextRunAt        time.Time
	LastStatus       string
	LastErrorCode    string
	LastErrorMessage string
	UpdatedAt        time.Time
	TaskName         string
}
