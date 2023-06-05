package model

import (
	engineModel "github.com/ralf-life/engine/pkg/model"
	"time"
)

type Flow struct {
	FlowID        string            `bson:"flow-id"`
	UserID        string            `bson:"user-id"`
	Name          string            `bson:"name"`
	Source        string            `bson:"source"`
	CacheDuration time.Duration     `bson:"cache-duration"`
	Flows         engineModel.Flows `bson:"flows"`
}
