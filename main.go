package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	papi "github.com/Go-routine-4595/oem-bridge/adapters/controller/api"
	"github.com/Go-routine-4595/oem-bridge/adapters/controller/broker"
	"github.com/Go-routine-4595/oem-bridge/adapters/gateway/display"
	event_hub "github.com/Go-routine-4595/oem-bridge/adapters/gateway/event-hub"
	pmqtt "github.com/Go-routine-4595/oem-bridge/adapters/gateway/mqtt"
	"github.com/Go-routine-4595/oem-bridge/model"
	"github.com/Go-routine-4595/oem-bridge/service"

	"github.com/Go-routine-4595/oem-bridge/middleware"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const (
	config  = "/opt/oem-bridge/config.yaml"
	version = 0.27
)

var CompileDate string

type Config struct {
	broker.ControllerConfig  `yaml:"ControllerConfig"`
	event_hub.EventHubConfig `yaml:"EventHubConfig"`
	pmqtt.MqttConf           `yaml:"MqttConfig"`
	papi.ApiConf             `yaml:"ApiConfig"`
	Duration                 int    `yaml:"Duration"`
	LogLevel                 int    `yaml:"LogLevel"`
	Type                     string `yaml:"Type"`
}

var logLevel map[int]string = map[int]string{
	-1: "trace",
	0:  "debug",
	1:  "info",
	2:  "warn",
	3:  "error",
	4:  "fatal",
	5:  "panic",
	6:  "disabled",
}

func main() {
	var (
		conf   Config
		svr    *broker.Controller
		svc    model.IService
		gtw    service.ISendAlarm
		eh     *event_hub.EventHub
		mqtt   *pmqtt.Mqtt
		api    *papi.Api
		wg     *sync.WaitGroup
		ctx    context.Context
		args   []string
		sig    chan os.Signal
		cancel context.CancelFunc
		err    error
	)

	args = os.Args

	fmt.Println("Starting oem-alarm v", version)
	fmt.Println(CompileDate)

	wg = &sync.WaitGroup{}

	if len(args) == 1 {
		fmt.Println("reading configuraiotn file: ", config)
		conf = openConfigFile(config)
	} else {
		fmt.Println("reading configuraiotn file: ", args[1])
		conf = openConfigFile(args[1])
	}

	// provide additional info for the confg/API
	conf.ApiConf.CompileDate = CompileDate
	conf.ApiConf.Version = fmt.Sprintf("%.2f", version)

	// log level
	log.Logger.With().Str("instanceId", "myid").Logger()
	log.Info().Msg("a message")
	zerolog.SetGlobalLevel(zerolog.InfoLevel + zerolog.Level(conf.LogLevel))
	conf.MqttConf.LogLevel = conf.LogLevel
	conf.EventHubConfig.LogLevel = conf.LogLevel
	conf.ControllerConfig.LogLevel = conf.LogLevel

	fmt.Printf("Log level: %s \n", logLevel[int(zerolog.InfoLevel+zerolog.Level(conf.LogLevel))])

	// duration of the service (exit after duration)
	if conf.Duration > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(conf.Duration)*time.Minute)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}

	// new gateway (display or eh)
	eh, err = event_hub.NewEventHub(ctx, wg, conf.EventHubConfig)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create event hub")
		mqtt, err = pmqtt.NewMqtt(ctx, wg, conf.MqttConf)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create mqtt")
			// or a Display if we fail to initiate a new event hub
			gtw = display.NewDisplay()
			// new service with simple display
			svc = service.NewService(gtw, conf.Type)
		} else {
			svc = service.NewService(mqtt, conf.Type)
		}

	} else {
		// new service with eh
		svc = service.NewService(eh, conf.Type)
	}

	// new middleware logger
	svc = middleware.NewLogger(conf.ControllerConfig, svc)
	// new controller with RabbitMQ connection
	svr = broker.NewController(conf.ControllerConfig, svc)

	// new Api
	api = papi.NewApi(conf.ApiConf)

	// start the Api
	api.Start(ctx, wg)

	// start the controller
	svr.Start(ctx, wg)

	sig = make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()
	// give 500 ms grace period to flush all logs
	time.Sleep(500 * time.Millisecond)
	wg.Wait()
}

func openConfigFile(s string) Config {
	if s == "" {
		s = "config.yaml"
	}

	f, err := os.Open(s)
	if err != nil {
		processError(errors.Join(err, errors.New("open config.yaml file")))
	}
	defer f.Close()

	var config Config
	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		processError(err)
	}
	return config

}

func processError(err error) {
	fmt.Println(err)
	os.Exit(2)
}
