package broker

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Go-routine-4595/oem-bridge/model"
	"github.com/rs/zerolog"
	"github.com/streadway/amqp"
)

type ControllerConfig struct {
	ConnectionString   string `yaml:"ConnectionString"`
	QueueName          string `yaml:"QueueName"`
	LogLevel           int    `yaml:"LogLevel"`
	Key                string `yaml:"Key"`
	CABundle           string `yaml:"CABundle"`
	Cert               string `yaml:"Cert"`
	InsecureSkipVerify bool   `yaml:"InsecureSkipVerify"`
}

type Controller struct {
	ConnectionString string
	QueueName        string
	Svc              model.IService
	logger           zerolog.Logger
	conn             *amqp.Connection
	channel          *amqp.Channel
	cfgTls           *tls.Config
	controllerType   string
	dialtls          bool
}

const reconnectInterval = 5 * time.Second

// NewController initializes and returns a new Controller instance with the specified configuration and service.
// It sets up a logger, loads TLS certificates, and handles errors with insecure skip verify as fallback.
func NewController(conf ControllerConfig, svc model.IService) *Controller {
	logger := initializeLogger(conf.LogLevel)

	btls := true
	tlsConfig, err := loadCert(conf)
	if err != nil {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
		btls = false
		logger.Error().Err(err).Msg("Failed to load CA certificate; using insecure skip verify")
	}
	if btls {
		logger.Debug().Msg("Checking certificates pool")
		listCertificates(tlsConfig.RootCAs, logger)
		logger.Debug().Msg("Checking certificates client")
		listCertificates(tlsConfig.ClientCAs, logger)
	}

	return &Controller{
		ConnectionString: conf.ConnectionString,
		QueueName:        conf.QueueName,
		Svc:              svc,
		cfgTls:           tlsConfig,
		logger:           logger,
		dialtls:          btls,
	}
}

// createLogger initializes and returns a new `zerolog.Logger` configured with the given log level.
// It sets the output to `os.Stdout` with RFC3339 time format and includes the process PID in the log context.
func initializeLogger(logLevel int) zerolog.Logger {
	return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		Level(zerolog.Level(logLevel+1)).
		With().
		Timestamp().
		Int("pid", os.Getpid()).
		Logger()
}

// loadCert loads and returns a configured tls.Config using the provided ControllerConfig for TLS settings.
// It reads the CA bundle, certificate, and key files specified in the config. If any file is missing, it returns an error.
// The function also handles loading X.509 key pairs and appending CA certificates to a new certificate pool.
// Note: InsecureSkipVerify is set to true regardless of the config setting.
func loadCert(conf ControllerConfig) (*tls.Config, error) {
	if conf.Key == "" || conf.Cert == "" || conf.CABundle == "" {
		return nil, fmt.Errorf("missing key, cert or ca bundle")
	}

	cert, err := tls.LoadX509KeyPair(conf.Cert, conf.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %v", err)
	}

	caCert, err := os.ReadFile(conf.CABundle)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA bundle: %v", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificates")
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}, nil
}

// connect establishes a connection to a RabbitMQ server using the Controller's connection string and queue name.
// It initializes a new AMQP connection and channel, and declares a durable queue.
// Returns an error if the connection, channel initialization, or queue declaration fails.
func (c *Controller) connect() error {
	var err error
	if c.dialtls {
		c.conn, err = amqp.DialTLS(c.ConnectionString, c.cfgTls)
	} else {
		c.conn, err = amqp.Dial(c.ConnectionString)
	}
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
	return err
}

// reconnect continuously attempts to re-establish a RabbitMQ connection until successful.
// It logs the status of each connection attempt and waits for a predefined interval before retrying.
func (c *Controller) reconnect() {
	for {
		c.logger.Info().Msg("Attempting to reconnect to RabbitMQ...")
		err := c.connect()
		if err == nil {
			c.logger.Info().Msg("Successfully reconnected to RabbitMQ...")
			break
		}
		c.logger.Warn().Err(err).Msg("Reconnect failed")
		time.Sleep(reconnectInterval)
	}
}

// Start begins the controller's operation by establishing a connection to RabbitMQ and starting message consumption.
// It runs the consume function in a separate goroutine, allowing it to process messages asynchronously.
// The method completes by signaling the associated WaitGroup when the operation is done or if a connection error occurs.
func (c *Controller) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	err := c.connect()
	if err != nil {
		c.logger.Fatal().Err(err).Caller().Msg("Failed to connect to RabbitMQ")
		return
	}
	go c.consume(ctx, wg)
}

// consume handles incoming messages from a RabbitMQ queue and processes them using the provided context and wait group.
// It continuously listens for messages, processes each message through the Svc.SendAlarm method, and acknowledges messages if successful.
// The method will attempt to reconnect if the connection or channel is closed, or if an error occurs during message consumption.
func (c *Controller) consume(ctx context.Context, wg *sync.WaitGroup) {
	var (
		connClose chan *amqp.Error
		chClose   chan *amqp.Error
		msgs      <-chan amqp.Delivery
		err       error
	)

	wg.Add(1)
	defer wg.Done()

	c.logger.Info().Msg("Waiting for events from RabbitMQ")

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

		connClose = make(chan *amqp.Error)
		chClose = make(chan *amqp.Error)
		c.conn.NotifyClose(connClose)
		c.channel.NotifyClose(chClose)

	loop:
		for {
			select {
			case msg := <-msgs:
				err = c.Svc.SendAlarm(msg.Body)
				if err != nil {
					c.logger.Error().Err(err).Msg("Failed to send alarm")
				} else {
					err = msg.Ack(false)
					if err != nil {
						c.logger.Error().Err(err).Msg("Failed to ack message")
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
				if errors.Is(ctx.Err(), context.Canceled) {
					c.logger.Warn().Msg("Closing RabbitMQ connection")
				} else {
					c.logger.Warn().Msg("Context deadline exceeded, closing RabbitMQ connection")
				}
				c.Close()
				return
			}
		}

		time.Sleep(reconnectInterval)
		c.logger.Warn().Msg("Channel closed, reconnecting...")
		c.reconnect()
	}
}

func (c *Controller) Close() error {
	err := c.channel.Close()
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to close channel")
	}

	err = c.conn.Close()
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to close connection")
	}

	return err
}
