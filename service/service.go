package service

import (
	"github.com/Go-routine-4595/oem-bridge/model"
	"github.com/rs/zerolog/log"
	"time"
)

type ISendAlarm interface {
	SendAlarm(events model.FCTSDataModel) error
}

type Service struct {
	gateway ISendAlarm
}

func NewService(g ISendAlarm) *Service {
	return &Service{
		gateway: g,
	}
}

func (s *Service) SendAlarm(value []byte) error {
	var (
		event model.FCTSDataModel
	)

	event = model.FCTSDataModel{
		SiteCode:   "NAMEM",
		TimeStamp:  time.Now().Unix(),
		SensorId:   "UAS-OEM-alarms",
		Uom:        "alarm",
		DataSource: "Honeywell",
		Value:      string(value),
	}

	log.Debug().Str("event", event.Value).Msg("sending alarm")

	return s.gateway.SendAlarm(event)
}
