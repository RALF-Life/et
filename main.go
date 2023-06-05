package main

import (
	"context"
	firebase "firebase.google.com/go"
	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	engineModel "github.com/ralf-life/engine/pkg/model"
	"github.com/ralf-life/et/internal/mongodb"
	gofiberfirebaseauth "github.com/sacsand/gofiber-firebaseauth"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/api/option"
	"log"
	"os"
)

type ReturnFlow engineModel.ReturnFlow

func (r *ReturnFlow) KeyIdentifier() string {
	return "return"
}

func (r *ReturnFlow) MarshalBSON() ([]byte, error) {
	return []byte("return"), nil
}

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
	_, err = app.Auth(context.TODO())
	if err != nil {
		log.Fatalln("error initializing auth:", err)
		return
	}

	// http server
	httpApp := fiber.New(fiber.Config{
		AppName: "E. T.",
	})
	// unauthorized routes here
	httpApp.Get("/world", func(ctx *fiber.Ctx) error {
		return ctx.SendString("world hello!")
	})
	httpApp.Use(gofiberfirebaseauth.New(gofiberfirebaseauth.Config{
		FirebaseApp: app,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return ctx.Status(fiber.StatusBadRequest).SendString(err.Error())
		},
	}))
	// authorized routes here
	httpApp.Get("/hello", func(ctx *fiber.Ctx) error {
		return ctx.SendString("hello world!")
	})
	if err = httpApp.Listen(":8081"); err != nil {
		panic(err)
	}

	// connect to mongo
	mongoClient, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = mongoClient.Disconnect(context.TODO())
	}()
	m := mongodb.New(mongoClient, dbName)
	if err = m.CreateIndexes(); err != nil {
		panic(err)
	}
	/*
		fmt.Println(m.FlowCollection().UpdateOne(context.TODO(),
			bson.M{
				"id": f.UserID,
			},
			bson.M{
				"$set": f,
			},
			options.Update().SetUpsert(true)))
	*/
}
