package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/Go-routine-4595/oem-bridge/adapters/gateway/display"
	event_hub "github.com/Go-routine-4595/oem-bridge/adapters/gateway/event-hub"
	"github.com/Go-routine-4595/oem-bridge/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Go-routine-4595/oem-bridge/adapters/controller"
	papi "github.com/Go-routine-4595/oem-bridge/adapters/controller/api"
	"github.com/Go-routine-4595/oem-bridge/adapters/controller/broker"
	"github.com/Go-routine-4595/oem-bridge/model"
	"github.com/Go-routine-4595/oem-bridge/service"

	_ "github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type Config struct {
	controller.ControllerConfig `yaml:"ControllerConfig"`
	event_hub.EventHubConfig    `yaml:"EventHubConfig"`
	Duration                    int `yaml:"Duration"`
}

func main() {
	var (
		conf   Config
		svr    *broker.Controller
		svc    model.IService
		gtw    service.ISendAlarm
		eh     *event_hub.EventHub
		api    *papi.Api
		wg     *sync.WaitGroup
		ctx    context.Context
		args   []string
		sig    chan os.Signal
		cancel context.CancelFunc
		err    error
	)

	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	args = os.Args

	wg = &sync.WaitGroup{}

	if len(args) == 1 {
		conf = openConfigFile("config2.yaml")
	} else {
		conf = openConfigFile(args[1])
	}

	if conf.Duration > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(conf.Duration)*time.Minute)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}

	// new gateway (display or eh)
	eh, err = event_hub.NewEventHub(ctx, wg, conf.EventHubConfig)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create event hub")
		// or a Display if we fail to initiate a new event hub
		gtw = display.NewDisplay()
		// new service with simple display
		svc = service.NewService(gtw)
	} else {
		// new service with eh
		svc = service.NewService(eh)
	}

	// new middleware logger
	svc = middleware.NewLogger(conf.ControllerConfig, svc)
	// new controller
	svr = broker.NewController(conf.ControllerConfig, svc)

	// new Api
	api = papi.NewApi(conf.ControllerConfig)

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
