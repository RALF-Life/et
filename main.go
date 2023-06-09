package main

import (
	"context"
	firebase "firebase.google.com/go"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	engineModel "github.com/ralf-life/engine/pkg/model"
	"github.com/ralf-life/et/internal/mongodb"
	"github.com/ralf-life/et/pkg/model"
	gofiberfirebaseauth "github.com/ralf-life/gofiber-firebaseauth"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/api/option"
	"log"
	"os"
	"time"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// load environmental variables
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("You must set your 'MONGODB_URI' environmental variable. " +
			"See\n\t https://www.mongodb.com/docs/drivers/go/current/usage-examples/#environment-variable")
		return
	}
	dbName := os.Getenv("MONGODB_DB")
	if dbName == "" {
		log.Fatal("You mus set your 'MONGODB_DB' environmental variable.")
		return
	}
	fbCredFile := os.Getenv("FIREBASE_CREDENTIALS")
	if fbCredFile == "" {
		log.Fatal("You mus set your 'FIREBASE_CREDENTIALS' environmental variable.")
		return
	}

	// connect firebase auth
	opt := option.WithCredentialsFile(fbCredFile)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalln("error initializing firebase app:", err)
		return
	}
	auth, err := app.Auth(context.TODO())
	if err != nil {
		log.Fatalln("error initializing auth:", err)
		return
	}
	fmt.Println("verify:")
	fmt.Println(auth.VerifyIDToken(context.TODO(), "eyJhbGciOiJSUzI1NiIsImtpZCI6IjU0NWUyNDZjNTEwNmExMGQ2MzFiMTA0M2E3MWJiNTllNWJhMGM5NGQiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL3NlY3VyZXRva2VuLmdvb2dsZS5jb20vcmFsZi1kZW1vIiwiYXVkIjoicmFsZi1kZW1vIiwiYXV0aF90aW1lIjoxNjg1OTU2MjA4LCJ1c2VyX2lkIjoicTY5eTdweVJ2emJZT3R4OHVCQmc4Zkc4d2dOMiIsInN1YiI6InE2OXk3cHlSdnpiWU90eDh1QkJnOGZHOHdnTjIiLCJpYXQiOjE2ODU5NTYyMDgsImV4cCI6MTY4NTk1OTgwOCwiZW1haWwiOiJ0ZXN0MkB0ZXN0LmNvbSIsImVtYWlsX3ZlcmlmaWVkIjpmYWxzZSwiZmlyZWJhc2UiOnsiaWRlbnRpdGllcyI6eyJlbWFpbCI6WyJ0ZXN0MkB0ZXN0LmNvbSJdfSwic2lnbl9pbl9wcm92aWRlciI6InBhc3N3b3JkIn19.iB0M5MZnv38UmmrxGLoH4xJdcadkKFHKEoZlT77BRL7erAXs9spYRlUZVJKVp7MAQrwmMUtzuoau4uPHWEa7sHQqo_qQBSP9QqDAZoUyDDTAiB13y00ezNx2EPha47LUj3TJJVHi8xGjQ_qoV093Q2IpoLfrPJGKXjwsT6nwp1X7Z9svxJK5RL6KIb356lxsH6HQri-hGRU-OKROMbEIVv3CmTYg4ieUlXDP7NCOUftch89KdNT4XMdGDONg_yhVjjC7nXfGNrbWc9MHoNmEY9R6UN4tRNVkkMS__i_iLzxltI_XnLRXjNSvHH2jcECaet-_qd5ofm7vG7cooUVPgw"))

	// http server
	httpApp := fiber.New(fiber.Config{
		AppName: "E. T.",
	})
	// unauthorized routes here
	httpApp.Get("/world", func(ctx *fiber.Ctx) error {
		return ctx.SendString("world hello!")
	})

	// connect to mongo
	mongoClient, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = mongoClient.Disconnect(context.TODO())
	}()
	m := mongodb.New(mongoClient, dbName)
	/*
		if err = m.CreateIndexes(); err != nil {
			fmt.Println("index err")
			panic(err)
		}
	*/

	f := model.Flow{
		FlowID:        uuid.New().String(),
		UserID:        "wAFGXfTZsIeQO597GRbt4gDZQEl2",
		Name:          "My first Flow",
		Source:        "https://google.com",
		CacheDuration: 2 * time.Minute,
		Flows: engineModel.Flows{
			&engineModel.ActionFlow{
				FlowIdentifier: "filters/filter-out",
			},
			&engineModel.ConditionFlow{
				Condition: engineModel.Conditions{
					"Event.Summary() contains 'Test'",
				},
				Operator: "and",
				Then: engineModel.Flows{
					&engineModel.DebugFlow{
						Debug: "Event summary contains Test!",
					},
					&engineModel.ReturnFlow{
						Return: true,
					},
				},
				Else: engineModel.Flows{
					&engineModel.ConditionFlow{
						Condition: engineModel.Conditions{
							"Date.IsToday()",
						},
						Operator: "or",
						Else: engineModel.Flows{
							&engineModel.ActionFlow{
								FlowIdentifier: "filters/filter-out",
							},
							&engineModel.ReturnFlow{
								Return: true,
							},
						},
					},
				},
			},
		},
	}
	fmt.Println(m.FlowCollection().UpdateOne(context.TODO(),
		bson.M{
			"flow-id": f.FlowID,
		},
		bson.M{
			"$set": f,
		},
		options.Update().SetUpsert(true)))

	httpApp.Get("/flows/:flow_id", func(ctx *fiber.Ctx) error {
		flowID := ctx.Params("flow_id")
		result := m.FlowCollection().FindOne(context.TODO(), bson.M{
			"flow-id": flowID,
		})
		if result.Err() != nil {
			return fmt.Errorf("cannot find flow: %v", result.Err())
		}
		var f model.Flow
		if err := result.Decode(&f); err != nil {
			return fmt.Errorf("cannot decode flow: %v", err)
		}
		return ctx.Status(200).JSON(f)
	})

	httpApp.Use(gofiberfirebaseauth.New(app, gofiberfirebaseauth.Config{
		TokenExtractor: gofiberfirebaseauth.NewHeaderExtractor("Bearer "),
	}))

	// GET /me/flows - Returns a list of flow ids + names
	// GET /flows/:flow_id/json - Returns a flow as JSON
	// GET /flows/:flow_id - Executes the flow
	// POST /flows/:flow_id - Saves a flow

	// authorized routes here
	httpApp.Get("/hello", func(ctx *fiber.Ctx) error {
		return ctx.SendString("hello world!")
	})

	if err = httpApp.Listen(":8081"); err != nil {
		panic(err)
	}
}
