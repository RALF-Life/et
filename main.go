package main

import (
	"context"
	"errors"
	firebase "firebase.google.com/go"
	"fmt"
	ics "github.com/darmiel/golang-ical"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/patrickmn/go-cache"
	"github.com/ralf-life/engine/pkg/engine"
	engineModel "github.com/ralf-life/engine/pkg/model"
	"github.com/ralf-life/et/internal/mongodb"
	"github.com/ralf-life/et/pkg/model"
	gofiberfirebaseauth "github.com/ralf-life/gofiber-firebaseauth"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/api/option"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const MaxContentLength = 100 * 1000 * 1000 // 100 MB

var (
	client                   = &http.Client{}
	ch                       = cache.New(5*time.Minute, 10*time.Minute)
	ErrExceededContentLength = errors.New("exceeded max. content length of " + strconv.Itoa(MaxContentLength))
)

func getSourceWithRequest(url string, cacheDuration time.Duration) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.ContentLength > MaxContentLength {
		return "", ErrExceededContentLength
	}
	valBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	val := string(valBytes)

	ch.Set("source::"+url, val, cacheDuration)
	fmt.Println("[" + url + "] from request")
	return val, nil
}

func getSource(url string, cacheDuration time.Duration) (string, error) {
	res, ok := ch.Get("source::" + url)
	if !ok {
		return getSourceWithRequest(url, cacheDuration)
	}
	fmt.Println("[" + url + "] from cache")
	return res.(string), nil
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

	// connect to mongo
	mongoClient, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = mongoClient.Disconnect(context.TODO())
	}()
	m := mongodb.New(mongoClient, dbName)

	// http server
	httpApp := fiber.New(fiber.Config{
		AppName: "E. T.",
	})

	httpApp.Get("/:flow_id.json", func(ctx *fiber.Ctx) error {
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
		fmt.Printf("%+v\n", f)
		// a user can only show flows which he has access to
		u := ctx.Locals("user").(gofiberfirebaseauth.User)
		if u.UserID != f.UserID {
			return ctx.Status(http.StatusUnauthorized).SendString("you are not allowed to access this flow.")
		}
		return ctx.Status(200).JSON(f)
	})

	httpApp.Get("/:flow_id.ics", func(ctx *fiber.Ctx) error {
		verbose := ctx.QueryBool("verbose", false)
		debug := ctx.QueryBool("debug", true)

		flowID := ctx.Params("flow_id")
		result := m.FlowCollection().FindOne(context.TODO(), bson.M{
			"flow-id": flowID,
		})
		if result.Err() != nil {
			return fmt.Errorf("cannot find flow: %v", result.Err())
		}
		var flow model.Flow
		if err := result.Decode(&flow); err != nil {
			return fmt.Errorf("cannot decode flow: %v", err)
		}

		// validate profile
		if strings.TrimSpace(flow.Source) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "`source` required")
		}

		// require a cache duration of at least 120s
		cd := time.Duration(flow.CacheDuration)
		if cd.Minutes() < 2.0 {
			cd = 2 * time.Minute
		}

		body, err := getSource(flow.Source, cd)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "cannot request source ("+err.Error()+")")
		}

		// parse calendar
		cal, err := ics.ParseCalendar(strings.NewReader(body))
		if err != nil {
			return fiber.NewError(fiber.StatusExpectationFailed, "failed to parse source calendar ("+err.Error()+")")
		}

		// create context and run flow
		cp := &engine.ContextFlow{
			Profile: &engineModel.Profile{
				Name:          flow.Name,
				Source:        flow.Source,
				CacheDuration: engineModel.Duration(flow.CacheDuration),
				Flows:         flow.Flows,
			},
			Context:     make(map[string]interface{}),
			EnableDebug: debug,
			Verbose:     verbose,
		}
		if err = engine.ModifyCalendar(cp, flow.Flows, cal); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to run flow ("+err.Error()+")")
		}
		if r := recover(); r != nil {
			fmt.Printf("had to recover! %+v\n", r)
			return ctx.Status(http.StatusInternalServerError).SendString("recover")
		}

		// append debug messages as header
		ctx.Append("X-Debug-Message-Count", strconv.Itoa(len(cp.Debugs)))
		for i, v := range cp.Debugs {
			ctx.Append(fmt.Sprintf("X-Debug-Message-%d", i+1), fmt.Sprintf("%+v", v))
		}

		// append content-type and return calendar
		ctx.Set("Content-Type", "text/calendar")
		return ctx.Status(http.StatusOK).SendString(cal.Serialize())
	})

	httpApp.Use(gofiberfirebaseauth.New(app, gofiberfirebaseauth.Config{
		TokenExtractor: gofiberfirebaseauth.NewHeaderExtractor("Bearer "),
	}))

	// GET /me/flows - Returns a list of flow ids + names
	httpApp.Get("/flows", func(ctx *fiber.Ctx) error {
		u := ctx.Locals("user").(gofiberfirebaseauth.User)
		cur, err := m.FlowCollection().Find(context.TODO(), bson.M{
			"user-id": u.UserID,
		})
		if err != nil {
			return ctx.Status(http.StatusInternalServerError).SendString(err.Error())
		}
		var results []model.FlowHead
		if err = cur.All(context.TODO(), &results); err != nil {
			return ctx.Status(http.StatusInternalServerError).SendString(err.Error())
		}
		return ctx.Status(http.StatusOK).JSON(results)
	})

	// POST /flows - Saves/Creates a flow
	httpApp.Post("/flows", func(ctx *fiber.Ctx) error {
		var flow model.Flow
		if err := ctx.BodyParser(&flow); err != nil {
			return ctx.Status(http.StatusBadRequest).SendString(err.Error())
		}
		u := ctx.Locals("user").(gofiberfirebaseauth.User)
		flow.UserID = u.UserID

		// if no flow id was specified, generate a new one.
		if flow.FlowID == "" {
			flow.FlowID = uuid.New().String()
		}

		// min. cache duration of 2 minutes required.
		if flow.CacheDuration.Minutes() < 2 {
			flow.CacheDuration = 2 * time.Minute
		}

		result, err := m.FlowCollection().UpdateOne(context.TODO(),
			bson.M{
				"flow-id": flow.FlowID,
				"user-id": u.UserID,
			},
			bson.M{
				"$set": flow,
			},
			options.Update().SetUpsert(true))
		if err != nil {
			return ctx.Status(http.StatusInternalServerError).SendString(err.Error())
		}
		msg := fmt.Sprintf("matched %d, modified %d, upserted %d",
			result.MatchedCount, result.ModifiedCount, result.UpsertedCount)
		if result.UpsertedCount > 0 {
			ctx = ctx.Status(http.StatusCreated)
		} else {
			ctx = ctx.Status(http.StatusOK)
		}
		return ctx.SendString(msg)
	})

	if err = httpApp.Listen(":8081"); err != nil {
		panic(err)
	}
}
