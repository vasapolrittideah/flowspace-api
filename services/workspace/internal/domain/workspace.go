package domain

import "time"

type Workspace struct {
	ID        string
	Name      string
	CreatedAt time.Time
}
