package mongodb

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	FlowCollection = "flows"
)

type Client struct {
	database string
	client   *mongo.Client
}

func New(client *mongo.Client, database string) *Client {
	return &Client{
		database: database,
		client:   client,
	}
}

func (c *Client) collection(name string) *mongo.Collection {
	return c.client.Database(c.database).Collection(name)
}

func (c *Client) FlowCollection() *mongo.Collection {
	return c.collection(FlowCollection)
}

func (c *Client) CreateIndexes() (err error) {
	if _, err = c.FlowCollection().Indexes().CreateOne(context.TODO(),
		mongo.IndexModel{
			Keys:    bson.M{"flow-id": 1},
			Options: options.Index().SetUnique(true),
		}); err != nil {
		return
	}
	// ...
	return
}
