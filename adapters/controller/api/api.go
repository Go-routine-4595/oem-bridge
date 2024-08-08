package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Go-routine-4595/oem-bridge/adapters/controller"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	_ "github.com/Go-routine-4595/oem-bridge/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// API swagger doc generation
// https://github.com/swaggo/swag?tab=readme-ov-file#getting-started
// https://github.com/swaggo/swag?tab=readme-ov-file#declarative-comments-format
// command to generate doc after update
// swag init -g ./adapters/controller/api/api.go -o docs

type Api struct {
	MgtUrl    string
	QueueName string
	Port      int
	logger    zerolog.Logger
}

// QueueInfo represents the JSON structure returned by RabbitMQ API
type QueueInfo struct {
	Messages               int `json:"messages"`
	MessagesReady          int `json:"messages_ready"`
	MessagesUnacknowledged int `json:"messages_unacknowledged"`
}

func NewApi(conf controller.ControllerConfig) *Api {
	return &Api{
		MgtUrl:    conf.MgtUrl,
		QueueName: conf.QueueName,
		Port:      conf.Port,
		//logger:    zerolog.New(os.Stdout).Level(zerolog.Level(conf.LogLevel + 1)).With().Timestamp().Caller().Logger(),
		logger: zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).Level(zerolog.Level(conf.LogLevel + 1)).With().Timestamp().Logger(),
	}
}

func (a *Api) Start(ctx context.Context, wg *sync.WaitGroup) {
	go a.start(ctx, wg)
}

// @title   Metrics API
// @version  1.0
// @description API for Metrics

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host   www.zogfestiv.store
// @BasePath  /

// @schemes http
func (a *Api) start(ctx context.Context, wg *sync.WaitGroup) {
	var (
		router *gin.Engine
		v1     *gin.RouterGroup
		server *http.Server
		err    error
	)

	wg.Add(1)

	router = gin.Default()

	server = &http.Server{
		Addr:    ":" + fmt.Sprint(a.Port),
		Handler: router,
	}

	//docs.SwaggerInfo.BasePath = "/api/v1"
	v1 = router.Group("/api/v1")
	{
		v1.GET("/metrics", a.Metrics)
	}
	//router.GET("/swagger/any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	//url := ginSwagger.URL("http://localhost:8080/docs/swagger.json") // The url pointing to API definition
	//router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, url))

	router.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/readyz", func(c *gin.Context) { c.Status(http.StatusOK) })

	go func() {
		if err = server.ListenAndServe(); err != nil {
			if errors.Is(http.ErrServerClosed, err) {
				a.logger.Err(err).Msg("Server closed under request")
			} else {
				a.logger.Err(err).Msg("Server closed unexpect")
			}
		}
	}()

	a.logger.Info().Msg("Waiting")
	<-ctx.Done()
	if err = server.Shutdown(context.Background()); err != nil {
		a.logger.Err(err).Msg("Server close")
	}
	a.logger.Info().Msg("Shutting down the api")
	wg.Done()

}

// Metrics godoc
// @BasePath 	/api/v1
// PingExample 	godoc
// @Summary 	metrics
// @Schemes
// @Description provides RabbitMQ metrics
// @Tags 		example
// @Accept 		json
// @Produce 	json
// @Success 	200 	{object} QueueInfo
// @Router 		/metrics [get]
func (a *Api) Metrics(c *gin.Context) {
	// Replace with your RabbitMQ management API endpoint and credentials
	url := a.MgtUrl + "/api/queues/%2F/" + a.QueueName

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		a.logger.Fatal().Err(err).Msg("Failed to create request")
	}

	req.SetBasicAuth("guest", "guest") // RabbitMQ default username and password

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		a.logger.Fatal().Err(err).Msg("Failed to make request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		a.logger.Fatal().Err(err).Msg("Unexpected status code")
	}

	var queueInfo QueueInfo
	if err := json.NewDecoder(resp.Body).Decode(&queueInfo); err != nil {
		a.logger.Fatal().Err(err).Msg("Failed to decode response")
	}

	a.logger.Debug().Int("messages", queueInfo.Messages).Msg("Messages in queue")
	a.logger.Debug().Int("messages_ready", queueInfo.MessagesReady).Msg("Messages in queue")
	a.logger.Debug().Int("messages_unacknowledged", queueInfo.MessagesUnacknowledged).Msg("Messages unacknowledged")

	c.JSON(http.StatusOK, resp.Body)
}
