package controller

type ControllerConfig struct {
	ConnectionString string `yaml:"ConnectionString"`
	QueueName        string `yaml:"QueueName"`
	LogLevel         int    `yaml:"LogLevel"`
	MgtUrl           string `yaml:"MgtUrl"`
	Port             int    `yaml:"Port"`
	CompileDate      string
	Version          string
}
