package mongodb

import (
	"go.mongodb.org/mongo-driver/mongo"
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
