package display

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Go-routine-4595/oem-bridge/model"
)

type Display struct{}

func NewDisplay() *Display {
	return &Display{}
}

func (d *Display) SendAlarm(events model.FCTSDataModel) error {
	var (
		buf []byte
		err error
	)

	buf, err = json.Marshal(events)
	if err != nil {
		return errors.Join(err, errors.New("failed to marshal event display.CreateAlarm"))
	}

	display(string(buf))

	return nil
}

func display(text string) {
	fmt.Println(text)
}
