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

func (s *Service) TestAlarm(value []byte) error {
	return nil
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

	log.Trace().Str("event", event.Value).Msg("sending alarm")
	// just to output the message for documentation purpose
	// tmp, _ := json.Marshal(event)
	// fmt.Println(string(tmp))

	return s.gateway.SendAlarm(event)
}
