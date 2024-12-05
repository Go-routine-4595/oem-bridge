package display

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Go-routine-4595/oem-bridge/model"
)

type Display struct {
	outputFunc func(string)
}

// NewDisplay initializes a Display with a default print function
func NewDisplay() *Display {
	return &Display{
		outputFunc: printAlarm,
	}
}

// SendAlarmRaw sends a raw byte slice as an alarm
func (d *Display) SendAlarmRaw(b []byte) error {
	d.outputFunc(string(b))
	return nil
}

// SendAlarm marshals FCTSDataModel and sends it as an alarm
func (d *Display) SendAlarm(events model.FCTSDataModel) error {
	text, err := marshalEvent(events)
	if err != nil {
		return err
	}
	d.outputFunc(text)
	return nil
}

// marshalEvent converts the event data to a JSON string
func marshalEvent(events model.FCTSDataModel) (string, error) {
	buf, err := json.Marshal(events)
	if err != nil {
		return "", errors.Join(err, errors.New("failed to marshal event in display.SendAlarm"))
	}
	return string(buf), nil
}

// printAlarm prints the provided text to standard output
func printAlarm(text string) {
	fmt.Println(text)
}
