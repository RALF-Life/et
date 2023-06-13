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

type FlowHead struct {
	FlowID        string        `bson:"flow-id" json:"flow-id"`
	UserID        string        `bson:"user-id" json:"user-id"`
	Name          string        `bson:"name" json:"name"`
	Source        string        `bson:"source" json:"source"`
	CacheDuration time.Duration `bson:"cache-duration" json:"cache-duration"`
}
