package middleware

import (
	"github.com/Go-routine-4595/oem-bridge/adapters/controller"
	"github.com/Go-routine-4595/oem-bridge/model"
	"github.com/rs/zerolog"
	"os"
	"time"
)

type Logger struct {
	svc    model.IService
	logger zerolog.Logger
}

func NewLogger(conf controller.ControllerConfig, svc model.IService) *Logger {
	return &Logger{
		svc:    svc,
		logger: zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).Level(zerolog.Level(conf.LogLevel + 1)).With().Timestamp().Logger(),
		//logger:    zerolog.New(os.Stdout).Level(zerolog.Level(conf.LogLevel + 1)).With().Timestamp().Caller().Logger(),
	}
}

func (l *Logger) SendAlarm(events []byte) error {
	defer func(timeCalled time.Time) {
		l.logger.Info().Int64("duration", time.Now().Sub(timeCalled).Milliseconds()).Msg("Message sent in ms")
	}(time.Now())
	return l.svc.SendAlarm(events)
}
