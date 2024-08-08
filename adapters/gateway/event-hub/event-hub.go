package event_hub

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Go-routine-4595/oem-bridge/model"
	"github.com/rs/zerolog"
	"os"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs"
)

// connection string can have the event hub name like this
// Endpoint=sb://FctsNAMemNADevlEvtHub01.servicebus.windows.net/;SharedAccessKeyName=<KeyName>;SharedAccessKey=<KeyValue>;EntityPath=honeywell-uas-oem-alarms
// see https://learn.microsoft.com/en-us/azure/event-hubs/event-hubs-get-connection-string
// see https://azure.github.io/azure-sdk/golang_introduction.html
// see https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs#ConsumerClient.Close
// see https://github.com/Azure/azure-sdk-for-go?tab=readme-ov-file

type EventHubConfig struct {
	Connection   string `yaml:"connection"`
	EventHubName string `yaml:"EventHubName"`
}

type EventHub struct {
	producerClient *azeventhubs.ProducerClient
}

func NewEventHub(ctx context.Context, wg *sync.WaitGroup, conf EventHubConfig) (*EventHub, error) {
	var (
		err            error
		producerClient *azeventhubs.ProducerClient
		l              zerolog.Logger
	)
	l = zerolog.New(os.Stdout).Level(zerolog.InfoLevel).With().Timestamp().Caller().Logger()

	producerClient, err = azeventhubs.NewProducerClientFromConnectionString(conf.Connection, conf.EventHubName, nil)

	if err != nil {
		return nil, errors.Join(err, errors.New("failed to create producer client"))
	}

	go func() {
		<-ctx.Done()
		err = producerClient.Close(ctx)
		l.Info().Msg("Event Hub connection closed")
		if err != nil {
			l.Error().Err(err).Msg("failed to close producer client")
		}
		wg.Done()
	}()

	return &EventHub{
		producerClient: producerClient,
	}, nil
}

func (e EventHub) SendAlarm(events model.FCTSDataModel) error {
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
