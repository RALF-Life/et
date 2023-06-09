package main

import (
	"context"
	"errors"
	firebase "firebase.google.com/go"
	"fmt"
	ics "github.com/darmiel/golang-ical"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
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
	"math"
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
	// build flags
	// main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}
	version string
	commit  string
	date    string
)

type buildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

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
		AppName:                 "E. T.",
		EnableTrustedProxyCheck: false,
		ProxyHeader:             "X-Forwarded-For",
	})

	httpApp.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "*",
		AllowHeaders: "*",
	}))

	// Health check
	httpApp.Get("/icanhazralf", func(ctx *fiber.Ctx) error {
		return ctx.Status(http.StatusOK).JSON(buildInfo{
			Version: version,
			Commit:  commit,
			Date:    date,
		})
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
		err = engine.ModifyCalendar(cp, flow.Flows, cal)

		// try to add the history entry
		debugMessages := make([]string, len(cp.Debugs))
		for i, v := range cp.Debugs {
			debugMessages[i] = fmt.Sprintf("%+v", v)
		}
		h := model.History{
			FlowID:    flowID,
			Address:   ctx.IP(),
			Timestamp: time.Now(),
			Success:   err == nil,
			Debug:     debugMessages,
			Action:    "execute",
		}
		if _, err = m.HistoryCollection().InsertOne(context.TODO(), h); err != nil {
			fmt.Println("warn :: cannot save EXECUTE history:", err)
		}

		if err != nil {
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
		return ctx.Status(200).JSON(f)
	})

	httpApp.Use(gofiberfirebaseauth.New(app, gofiberfirebaseauth.Config{
		TokenExtractor: gofiberfirebaseauth.NewHeaderExtractor("Bearer "),
	}))

	httpApp.Get("/:flow_id/history", func(ctx *fiber.Ctx) error {
		limit := math.Max(1, math.Min(10000, float64(ctx.QueryInt("limit", 100))))
		flowID := ctx.Params("flow_id")

		// Fun Fact: We're actually leaking the IP Address of the user.
		// But since this is only a demo, we don't care :)
		cur, err := m.HistoryCollection().Find(context.TODO(), bson.M{
			"flow-id": flowID,
		}, options.Find().SetSort(bson.M{"timestamp": -1}).SetLimit(int64(limit)))
		if err != nil {
			return ctx.Status(http.StatusInternalServerError).SendString(err.Error())
		}

		history := make([]model.History, 1)
		if err = cur.All(context.TODO(), &history); err != nil {
			return fmt.Errorf("cannot decode: %+v", err)
		}

		return ctx.Status(http.StatusOK).JSON(history)
	})

	httpApp.Delete("/:flow_id", func(ctx *fiber.Ctx) error {
		flowID := ctx.Params("flow_id")
		result, err := m.FlowCollection().DeleteOne(context.TODO(), bson.M{
			"flow-id": flowID,
		})

		h := model.History{
			FlowID:    flowID,
			Address:   ctx.IP(),
			Timestamp: time.Now(),
			Success:   err == nil,
			Debug:     nil,
			Action:    "delete",
		}
		if _, err = m.HistoryCollection().InsertOne(context.TODO(), h); err != nil {
			fmt.Println("warn :: cannot save DELETE history:", err)
		}

		if err != nil {
			return fmt.Errorf("cannot delete flow: %+v", err)
		}
		return ctx.Status(http.StatusOK).SendString(fmt.Sprintf("deleted %d",
			result.DeletedCount))
	})

	// GET /me/flows - Returns a list of flow ids + names
	httpApp.Get("/flows", func(ctx *fiber.Ctx) error {
		u := ctx.Locals("user").(gofiberfirebaseauth.User)
		cur, err := m.FlowCollection().Find(context.TODO(), bson.M{
			"user-id": u.UserID,
		})
		if err != nil {
			return ctx.Status(http.StatusInternalServerError).SendString(err.Error())
		}
		var results []model.Flow
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

		var msg string
		if result != nil {
			msg = fmt.Sprintf("matched %d, modified %d, upserted %d",
				result.MatchedCount, result.ModifiedCount, result.UpsertedCount)
		}

		h := model.History{
			FlowID:    flow.FlowID,
			Address:   ctx.IP(),
			Timestamp: time.Now(),
			Success:   err == nil,
			Debug:     []string{msg},
			Action:    "update",
		}
		if _, err = m.HistoryCollection().InsertOne(context.TODO(), h); err != nil {
			fmt.Println("warn :: cannot save UPDATE history:", err)
		}

		if err != nil {
			return ctx.Status(http.StatusInternalServerError).SendString(err.Error())
		}
		if result.UpsertedCount > 0 {
			ctx = ctx.Status(http.StatusCreated)
		} else {
			ctx = ctx.Status(http.StatusOK)
		}
		return ctx.SendString(msg)
	})

	if err = httpApp.Listen(":5544"); err != nil {
		panic(err)
	}
}
