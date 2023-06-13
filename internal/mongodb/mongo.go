package mongodb

import (
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	FlowCollection    = "flows"
	HistoryCollection = "history"
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

func (c *Client) HistoryCollection() *mongo.Collection {
	return c.collection(HistoryCollection)
}
