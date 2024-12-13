package service

import (
	"github.com/Go-routine-4595/oem-bridge/model"
	"github.com/rs/zerolog/log"
	"time"
)

const (
	typeRaw           = "Raw"
	typeFctsDataModel = "FCTSDataModel"
)

type ISendAlarm interface {
	SendAlarm(events model.FCTSDataModel) error
	SendAlarmRaw(events []byte) error
}

type Service struct {
	gateway ISendAlarm
	Type    string
}

func NewService(g ISendAlarm, t string) *Service {
	return &Service{
		gateway: g,
		Type:    t,
	}
}

func (s *Service) TestAlarm(value []byte) error {
	return nil
}

func (s *Service) SendAlarm(value []byte) error {
	var (
		event model.FCTSDataModel
	)

	if s.Type == typeFctsDataModel {
		event = model.FCTSDataModel{
			SiteCode:    "NAMEM",
			SensorId:    "NAMEM-UAS-OEM-alarms-test",
			DataSource:  "Honeywell simulation",
			TimeStamp:   time.Now().Unix(),
			Value:       string(value),
			Uom:         "OEM alarm",
			Quality:     "",
			Annotations: nil,
		}

		log.Trace().Str("event", event.Value).Msg("sending alarm")
		// just to output the message for documentation purpose
		// tmp, _ := json.Marshal(event)
		// fmt.Println(string(tmp))

		return s.gateway.SendAlarm(event)
	}
	if s.Type == typeRaw {
		log.Trace().Str("event", string(value)).Msg("sending alarm")
		return s.gateway.SendAlarmRaw(value)

	}
	log.Error().Msg("unknown type")
	return nil
}
