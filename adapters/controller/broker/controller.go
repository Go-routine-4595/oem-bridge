package broker

import (
	"context"
	"crypto/tls"
	"os"
	"sync"
	"time"

	"github.com/Go-routine-4595/oem-bridge/adapters/controller"
	"github.com/Go-routine-4595/oem-bridge/model"

	"github.com/rs/zerolog"
	"github.com/streadway/amqp"
)

type Controller struct {
	ConnectionString string
	QueueName        string
	Svc              model.IService
	logger           zerolog.Logger
	MgtUrl           string
	conn             *amqp.Connection
	channel          *amqp.Channel
	cfg              *tls.Config
}

func NewController(conf controller.ControllerConfig, svc model.IService) *Controller {
	return &Controller{
		ConnectionString: conf.ConnectionString,
		QueueName:        conf.QueueName,
		Svc:              svc,
		MgtUrl:           conf.MgtUrl,
		logger:           zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).Level(zerolog.Level(conf.LogLevel+1)).With().Timestamp().Int("pid", os.Getpid()).Logger(),
		//logger: zerolog.New(os.Stdout).Level(zerolog.Level(zerolog.DebugLevel)).With().Timestamp().Logger(),
	}
}

// loadCert load CA certificate to connect to AMQP server with authentication
func (c *Controller) loadCert() error {
	c.cfg = &tls.Config{
		InsecureSkipVerify: true,
	}
	return nil
}

// connect establishes a new connection and channel
func (c *Controller) connect() error {
	var err error

	// c.conn, err = amqp.Dial(c.ConnectionString)
	c.conn, err = amqp.DialTLS(c.ConnectionString, c.cfg)
	if err != nil {
		return err
	}

	c.channel, err = c.conn.Channel()
	if err != nil {
		return err
	}

	_, err = c.channel.QueueDeclare(
		c.QueueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return err
	}

	return nil
}

// reconnect handles reconnection logic
func (c *Controller) reconnect() {
	for {
		c.logger.Info().Msg("Attempting to reconnect to RabbitMQ...")
		err := c.connect()
		if err == nil {
			c.logger.Info().Msg("Successfully reconnected to RabbitMQ...")
			break
		}
		c.logger.Warn().Err(err).Msg("Reconnect failed")
		time.Sleep(5 * time.Second) // Exponential backoff could be implemented here
	}
}

// Start the controller
func (c *Controller) Start(ctx context.Context, wg *sync.WaitGroup) {
	var err error

	err = c.loadCert()
	if err != nil {
		c.logger.Fatal().Err(err).Caller().Msg("Failed to load CA certificate")
	}
	err = c.connect()
	if err != nil {
		c.logger.Fatal().Err(err).Caller().Msg("Failed to connect to RabbitMQ")
	}
	go c.consume(ctx, wg)
}

// consume starts the consumption of messages
func (c *Controller) consume(ctx context.Context, wg *sync.WaitGroup) {

	var (
		connClose chan *amqp.Error
		chClose   chan *amqp.Error
		msgs      <-chan amqp.Delivery
		err       error
	)

	wg.Add(1)
	c.logger.Info().Msg("Waiting event from RabbitMQ")

	for {
		msgs, err = c.channel.Consume(
			c.QueueName,
			"oem-bridge", // consumer
			false,        // auto-ack
			false,        // exclusive
			false,        // no-local
			false,        // no-wait
			nil,          // args
		)
		if err != nil {
			c.logger.Error().Err(err).Msg("Failed to register a consumer")
			c.reconnect()
			continue
		}

		// Create channels for detecting connection and channel closures
		connClose = make(chan *amqp.Error)
		chClose = make(chan *amqp.Error)

		c.conn.NotifyClose(connClose)
		c.channel.NotifyClose(chClose)

		// Listen for messages and processing:
		// - disconnection detection
		// - termination detection

	loop:
		for {
			select {
			case msg := <-msgs:
				// Process the message here
				err = c.Svc.SendAlarm(msg.Body)
				//err = c.Svc.TestAlarm(msg.Body)
				if err != nil {
					// failed to send the process message by the service, we don't ack the message, we don't
					// want to lose the data
					c.logger.Error().Err(err).Msg("Failed to send alarm")
				} else {
					err = msg.Ack(false)
					if err != nil {
						// failed to ack the message something bad happen, re-initialize the connection
						// hopefully the message is not lost and will be processed one connection re-initialized
						c.logger.Error().Err(err).Msg("Failed to ack message")
						//c.Close()
						break loop
					}
				}

			case err = <-connClose:
				c.logger.Warn().Err(err).Msg("Connection closed")
				break loop

			case err = <-chClose:
				c.logger.Warn().Err(err).Msg("Channel closed")
				break loop

			case <-ctx.Done():
				c.logger.Warn().Msg("Closing RabbitMQ connection")
				c.Close()
				wg.Done()
				return
			}
		}

		// Retry after closing
		time.Sleep(5 * time.Second)
		c.logger.Warn().Msg("Channel closed, reconnecting...")
		c.reconnect()

	}

}

// Close gracefully shuts down the connection and channel
func (c *Controller) Close() error {

	err := c.channel.Close()
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to close channel")
		return err
	}

	err = c.conn.Close()
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to close connection")
		return err
	}

	return nil
}
