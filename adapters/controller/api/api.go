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

// Constants for common endpoint paths
const (
	HealthEndpoint   = "/healthz"
	ReadyEndpoint    = "/readyz"
	SwaggerEndpoint  = "/swagger/*any"
	ApiV1BasePath    = "/api/v1"
	MetricsEndpoint  = "/metrics"
	InfoEndpoint     = "/info"
	RabbitMQEndpoint = "api/queues/%2F/"
	DefaultUser      = "guest"
	DefaultPassword  = "guest"
)

// ApiConf Define configuration for the API, using yaml tagged fields for configuration parsing
type ApiConf struct {
	MgtUrl      string `yaml:"MgtUrl"`
	Port        int    `yaml:"Port"`
	CompileDate string
	Version     string
	LogLevel    int    `yaml:"LogLevel"`
	QueueName   string `yaml:"QueueName"`
	UserName    string `yaml:"UserName"`
	Password    string `yaml:"Password"`
}

// Api provides server configuration details and handlers
type Api struct {
	MgtUrl    string
	QueueName string
	Port      int
	logger    zerolog.Logger
	UserName  string
	Password  string
}

// Info provides metadata about the server
type Info struct {
	CompileDate string `json:"compile_date"`
	Version     string `json:"version"`
	LogLevel    string `json:"log_level"`
	Date        string `json:"date"`
}

var info Info

// QueueInfo represents the JSON structure returned by RabbitMQ API
type QueueInfo struct {
	Messages               int `json:"messages"`
	MessagesReady          int `json:"messages_ready"`
	MessagesUnacknowledged int `json:"messages_unacknowledged"`
}

func NewApi(conf ApiConf) *Api {
	info = Info{
		CompileDate: conf.CompileDate,
		Version:     conf.Version,
		LogLevel:    fmt.Sprintf("%d", conf.LogLevel),
	}
	logger := zerolog.New(
		zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}).
		Level(zerolog.Level(conf.LogLevel+1)).
		With().
		Timestamp().
		Int("pid", os.Getpid()).
		Logger()

	return &Api{
		MgtUrl:    conf.MgtUrl,
		QueueName: conf.QueueName,
		Port:      conf.Port,
		logger:    logger,
		UserName:  conf.UserName,
		Password:  conf.Password,
	}
}

func (a *Api) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	router := a.setupRouter()
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.Port),
		Handler: router,
	}

	go a.runServer(server)

	a.logger.Info().Msg("Waiting API server ready")
	<-ctx.Done()

	a.shutdownServer(server, ctx)
}

func (a *Api) setupRouter() *gin.Engine {
	router := gin.Default()
	apiV1Group := router.Group(ApiV1BasePath)
	{
		apiV1Group.GET(MetricsEndpoint, a.Metrics)
		apiV1Group.GET(InfoEndpoint, a.Info)
	}

	router.GET(SwaggerEndpoint, ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET(HealthEndpoint, func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET(ReadyEndpoint, func(c *gin.Context) { c.Status(http.StatusOK) })

	return router
}

func (a *Api) runServer(server *http.Server) {
	if err := server.ListenAndServe(); err != nil {
		if errors.Is(http.ErrServerClosed, err) {
			a.logger.Warn().Err(err).Msg("Server closed under request")
		} else {
			a.logger.Err(err).Msg("Server closed unexpectedly")
		}
	}
}

func (a *Api) shutdownServer(server *http.Server, ctx context.Context) {
	switch ctx.Err() {
	case context.Canceled:
		a.logger.Warn().Msg("API server shutting down")
	case context.DeadlineExceeded:
		a.logger.Warn().Msg("API server shutting down on Context deadline exceeded")
	default:
		a.logger.Warn().Msg("API server shutting down; unknown reason")
	}

	if err := server.Shutdown(context.Background()); err != nil {
		a.logger.Err(err).Msg("Server close")
	}
}

// @title   Metrics API
// @version  1.0
// @description API for Metrics

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host   localhost:8090
// @BasePath  /api/v1/

// @schemes http

// Info godoc
// @BasePath 	/api/v1
// @Summary 	Info
// @Description provides server info
// @Tags 		example
// @Produce    json
// @Success    200 {object} Info
// @Router     /info [get]
func (a *Api) Info(c *gin.Context) {
	info.Date = time.Now().UTC().Format(time.RFC3339)
	c.JSON(http.StatusOK, info)
}

// Metrics godoc
// @BasePath 	/api/v1
// @Summary 	metrics
// @Description provides RabbitMQ metrics
// @Tags 		example
// @Accept 	    json
// @Produce    json
// @Success    200 {object} QueueInfo
// @Router     /metrics [get]
func (a *Api) Metrics(c *gin.Context) {
	url := a.MgtUrl + RabbitMQEndpoint + a.QueueName
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		a.logger.Fatal().Err(err).Msg("Failed to create request")
		return
	}
	req.SetBasicAuth(a.UserName, a.Password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "FMI/oem-bridge/metrics")
	req.Header.Set("X-Request-ID", c.Request.Header.Get("X-Request-ID"))
	req.Header.Set("X-Real-IP", c.ClientIP())
	req.Header.Set("X-Forwarded-For", c.ClientIP())
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", c.Request.Host)
	//req.Header.Set("X-Forwarded-Port", "443")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		a.logger.Err(err).Msg("Failed to make request")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		a.logger.Err(err).Msgf("Unexpected status code: %d %s", resp.StatusCode, resp.Status)
		c.JSON(resp.StatusCode, gin.H{"error": resp.Status})
		return
	}

	var queueInfo QueueInfo
	if err := json.NewDecoder(resp.Body).Decode(&queueInfo); err != nil {
		a.logger.Fatal().Err(err).Msg("Failed to decode response")
		return
	}

	a.logger.Debug().Int("messages", queueInfo.Messages).Msg("Messages in queue")
	a.logger.Debug().Int("messages_ready", queueInfo.MessagesReady).Msg("Messages ready in queue")
	a.logger.Debug().Int("messages_unacknowledged", queueInfo.MessagesUnacknowledged).Msg("Messages unacknowledged")
	c.JSON(http.StatusOK, queueInfo)
}
