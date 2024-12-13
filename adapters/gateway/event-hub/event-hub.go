package event_hub

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/Go-routine-4595/oem-bridge/model"
	"github.com/rs/zerolog"
	"os"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs"
)

// connection string can have the event hub name like this
// Endpoint=sb://FctsNAMemNADevlEvtHub01.servicebus.windows.net/;SharedAccessKeyName=<KeyName>;SharedAccessKey=<KeyValue>;EntityPath=honeywell-uas-oem-alarms
// see https://learn.microsoft.com/en-us/azure/event-hubs/event-hubs-get-connection-string
// see https://azure.github.io/azure-sdk/golang_introduction.html
// see https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs#ConsumerClient.Close
// see https://github.com/Azure/azure-sdk-for-go?tab=readme-ov-file

const (
	batchSize          = 50
	tickerDuration     = 200 * time.Millisecond
	defaultChannelSize = 100
	applicationID      = "oem-alarms"
)

var ErrorConnectionString error = errors.New("connection string must have the event hub name")

type EventHubConfig struct {
	Connection   string `yaml:"connection"`
	EventHubName string `yaml:"EventHubName"`
	LogLevel     int    `yaml:"LogLevel"`
}

type EventHub struct {
	producerClient *azeventhubs.ProducerClient
	ChanM          chan model.FCTSDataModel
	logger         zerolog.Logger
}

func NewEventHub(ctx context.Context, wg *sync.WaitGroup, conf EventHubConfig) (*EventHub, error) {
	var (
		err            error
		producerClient *azeventhubs.ProducerClient
		l              zerolog.Logger
		cfg            *tls.Config
		clientOptions  *azeventhubs.ProducerClientOptions
		eh             *EventHub
	)
	l = initializeLogger(conf.LogLevel)

	cfg = &tls.Config{
		InsecureSkipVerify: true,
	}
	clientOptions = &azeventhubs.ProducerClientOptions{
		ApplicationID: applicationID,
		TLSConfig:     cfg,
	}

	producerClient, err = azeventhubs.NewProducerClientFromConnectionString(conf.Connection, conf.EventHubName, clientOptions)

	if err != nil {
		tmp := errors.New("key \"Endpoint\" must not be empty")
		if err.Error() == tmp.Error() {
			return nil, ErrorConnectionString
		}
		return nil, errors.Join(err, errors.New("failed to create producer client"))
	}

	l.Info().Msg("Event Hub handler created")

	eh = &EventHub{
		producerClient: producerClient,
		ChanM:          make(chan model.FCTSDataModel, defaultChannelSize),
		logger:         l,
	}

	eh.start(ctx, wg)

	return eh, nil
}

func initializeLogger(logLevel int) zerolog.Logger {
	return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		Level(zerolog.InfoLevel+zerolog.Level(logLevel)).
		With().Timestamp().Int("pid", os.Getpid()).Logger()
}

func (e *EventHub) start(ctx context.Context, wg *sync.WaitGroup) {
	var (
		ticker *time.Ticker
		msg    model.FCTSDataModel
		mlist  []model.FCTSDataModel
	)

	wg.Add(1)
	ticker = time.NewTicker(tickerDuration)
	defer wg.Done()
	defer ticker.Stop()

	go func() {
		mlist = make([]model.FCTSDataModel, 0, batchSize)
		for {
			select {
			case <-ctx.Done():
				e.handleContextDone(ctx)
				return
			case <-ticker.C:
				ticker.Stop()
				// flush and send message every 200 millisecond don't wait the buffer to be full
				if len(mlist) > 0 {
					e.flushMessages(&mlist)
				}
				ticker.Reset(tickerDuration)
			case msg = <-e.ChanM:
				mlist = append(mlist, msg)
				if len(mlist) >= batchSize {
					e.flushMessages(&mlist)
				}

			}
		}
	}()
}

func (e *EventHub) flushMessages(mlist *[]model.FCTSDataModel) {
	if len(*mlist) > 0 {
		e.logger.Debug().Int("count", len(*mlist)).Msg("Sending batch of messages")
		if err := e.SendAlarmBatch(*mlist); err != nil {
			e.logger.Error().Err(err).Int("msize", len(*mlist)).Msg("Failed to send alarms")
		}
		*mlist = (*mlist)[:0]
	}
}

func (e *EventHub) handleContextDone(ctx context.Context) {
	msg := "Event Hub handler deleted"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		msg = "Event Hub handler deleted: context deadline exceeded"
	}
	e.logger.Warn().Msg(msg)

	if err := e.producerClient.Close(ctx); err != nil {
		e.logger.Warn().Err(err).Msg("Event Hub handler failed to close properly")
	}
}

func (e *EventHub) SendAlarmBatch(events []model.FCTSDataModel) error {
	var (
		buf             []byte
		err             error
		msg             *azeventhubs.EventData
		newBatchOptions *azeventhubs.EventDataBatchOptions
	)

	newBatchOptions = &azeventhubs.EventDataBatchOptions{
		// The options allow you to control the size of the batch, as well as the partition it will get sent to.

		// PartitionID can be used to target a specific partition ID.
		// specific partition ID.
		//
		// PartitionID: partitionID,

		// PartitionKey can be used to ensure that messages that have the same key
		// will go to the same partition without requiring your application to specify
		// that partition ID.
		//
		// PartitionKey: partitionKey,

		//
		// Or, if you leave both PartitionID and PartitionKey nil, the service will choose a partition.
	}

	// Creates an EventDataBatch, which you can use to pack multiple events together, allowing for efficient transfer.
	batch, err := e.producerClient.NewEventDataBatch(context.TODO(), newBatchOptions)
	if err != nil {
		return errors.Join(err, errors.New("failed to create event data batch"))
	}

	for _, event := range events {
		buf, err = json.Marshal(event)
		if err != nil {
			return errors.Join(err, errors.New("failed to marshal event display.CreateAlarm"))
		}

		msg = createEventForAlarm(buf)
	retry:
		err = batch.AddEventData(msg, nil)

		if errors.Is(err, azeventhubs.ErrEventDataTooLarge) {
			if batch.NumEvents() == 0 {
				// This one event is too large for this batch, even on its own. No matter what we do it
				// will not be sendable at its current size.
				return errors.Join(err, errors.New("failed to send alarm event is too large"))
			}

			// This batch is full - we can send it and create a new one and continue
			// packaging and sending events.
			if err = e.producerClient.SendEventDataBatch(context.TODO(), batch, nil); err != nil {
				return errors.Join(err, errors.New("failed to send alarm couldn't send the event"))
			}

			// create the next batch we'll use for events, ensuring that we use the same options
			// each time so all the messages go the same target.
			tmpBatch, err := e.producerClient.NewEventDataBatch(context.TODO(), newBatchOptions)

			if err != nil {
				return errors.Join(err, errors.New("failed to send alarm couldn't create a new batch"))
			}

			batch = tmpBatch

			// rewind so we can retry adding this event to a batch
			goto retry
		} else if err != nil {
			return errors.Join(err, errors.New("failed to send alarm"))
		}
	}

	// if we have any events in the last batch, send it
	if batch.NumEvents() > 0 {
		if err := e.producerClient.SendEventDataBatch(context.TODO(), batch, nil); err != nil {
			return errors.Join(err, errors.New("failed to send alarm couldn't send the event"))
		}
	}

	return nil
}

func (e *EventHub) SendAlarm(events model.FCTSDataModel) error {
	e.ChanM <- events
	e.logger.Trace().Int("chan", len(e.ChanM)).Msg("Event Hub handler channel status")
	return nil
}

func (e *EventHub) SendAlarmRaw(b []byte) error {
	e.logger.Panic().Msg("not available for Event Hub")
	return nil
}

func (e *EventHub) SendAlarmBak(events model.FCTSDataModel) error {
	var (
		mlist []model.FCTSDataModel
	)
	mlist = append(mlist, events)
	return e.SendAlarmBatch(mlist)
}

func (e *EventHub) oldSendAlarm(events model.FCTSDataModel) error {
	var (
		buf             []byte
		err             error
		msg             *azeventhubs.EventData
		newBatchOptions *azeventhubs.EventDataBatchOptions
	)

	buf, err = json.Marshal(events)
	if err != nil {
		return errors.Join(err, errors.New("failed to marshal event display.CreateAlarm"))
	}

	newBatchOptions = &azeventhubs.EventDataBatchOptions{
		// The options allow you to control the size of the batch, as well as the partition it will get sent to.

		// PartitionID can be used to target a specific partition ID.
		// specific partition ID.
		//
		// PartitionID: partitionID,

		// PartitionKey can be used to ensure that messages that have the same key
		// will go to the same partition without requiring your application to specify
		// that partition ID.
		//
		// PartitionKey: partitionKey,

		//
		// Or, if you leave both PartitionID and PartitionKey nil, the service will choose a partition.
	}

	// Creates an EventDataBatch, which you can use to pack multiple events together, allowing for efficient transfer.
	batch, err := e.producerClient.NewEventDataBatch(context.TODO(), newBatchOptions)
	if err != nil {
		return errors.Join(err, errors.New("failed to create event data batch"))
	}

	msg = createEventForAlarm(buf)

retry:
	err = batch.AddEventData(msg, nil)

	if errors.Is(err, azeventhubs.ErrEventDataTooLarge) {
		if batch.NumEvents() == 0 {
			// This one event is too large for this batch, even on its own. No matter what we do it
			// will not be sendable at its current size.
			return errors.Join(err, errors.New("failed to send alarm event is too large"))
		}

		// This batch is full - we can send it and create a new one and continue
		// packaging and sending events.
		if err = e.producerClient.SendEventDataBatch(context.TODO(), batch, nil); err != nil {
			return errors.Join(err, errors.New("failed to send alarm couldn't send the event"))
		}

		// create the next batch we'll use for events, ensuring that we use the same options
		// each time so all the messages go the same target.
		tmpBatch, err := e.producerClient.NewEventDataBatch(context.TODO(), newBatchOptions)

		if err != nil {
			return errors.Join(err, errors.New("failed to send alarm couldn't create a new batch"))
		}

		batch = tmpBatch

		// rewind so we can retry adding this event to a batch
		goto retry
	} else if err != nil {
		return errors.Join(err, errors.New("failed to send alarm"))
	}

	// if we have any events in the last batch, send it
	if batch.NumEvents() > 0 {
		if err := e.producerClient.SendEventDataBatch(context.TODO(), batch, nil); err != nil {
			return errors.Join(err, errors.New("failed to send alarm couldn't send the event"))
		}
	}

	return nil
}

func createEventForAlarm(buf []byte) *azeventhubs.EventData {
	return &azeventhubs.EventData{
		Body: buf,
	}
}
