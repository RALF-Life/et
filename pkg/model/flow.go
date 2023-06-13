package model

import (
	engineModel "github.com/ralf-life/engine/pkg/model"
	"time"
)

type Flow struct {
	FlowID        string            `bson:"flow-id" json:"flow-id"`
	UserID        string            `bson:"user-id" json:"user-id"`
	Name          string            `bson:"name" json:"name"`
	Source        string            `bson:"source" json:"source"`
	CacheDuration time.Duration     `bson:"cache-duration" json:"cache-duration"`
	Flows         engineModel.Flows `bson:"flows" json:"flows"`
}

type History struct {
	FlowID    string    `bson:"flow-id" json:"flow-id,omitempty"`
	Address   string    `bson:"address" json:"address,omitempty"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
	Success   bool      `bson:"success" json:"success,omitempty"`
	Debug     []string  `bson:"debug" json:"debug,omitempty"`
	Action    string    `bson:"action" json:"action,omitempty"` // updated | executed | deleted
}
