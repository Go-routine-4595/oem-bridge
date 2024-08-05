package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/Go-routine-4595/oem-bridge/adapters/gateway/display"
	"github.com/Go-routine-4595/oem-bridge/middleware"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/Go-routine-4595/oem-bridge/adapters/controller"
	papi "github.com/Go-routine-4595/oem-bridge/adapters/controller/api"
	"github.com/Go-routine-4595/oem-bridge/adapters/controller/broker"
	"github.com/Go-routine-4595/oem-bridge/model"
	"github.com/Go-routine-4595/oem-bridge/service"

	"gopkg.in/yaml.v3"
)

type Config struct {
	controller.ControllerConfig `yaml:"ControllerConfig"`
}

func main() {
	var (
		conf   Config
		svr    *broker.Controller
		svc    model.IService
		gtw    service.ISendAlarm
		api    *papi.Api
		wg     *sync.WaitGroup
		ctx    context.Context
		args   []string
		sig    chan os.Signal
		cancel context.CancelFunc
	)

	args = os.Args

	wg = &sync.WaitGroup{}
	ctx, cancel = context.WithCancel(context.Background())

	if len(args) == 1 {
		conf = openConfigFile("config.yaml")
	} else {
		conf = openConfigFile(args[1])
	}

	// new logger
	gtw = display.NewDisplay()
	// new service
	svc = service.NewService(gtw)
	// new logger
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
