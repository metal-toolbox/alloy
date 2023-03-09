package model

import (
	"errors"
	"os"
	"strings"
	"time"
)

var (
	defaultNatsStreamName               = "controllers"
	defaultNatsStreamURNNamespace       = "hollow-controllers"
	defaultNatsStreamPrefix             = "com.hollow.sh.controllers.replies"
	defaultNatsStreamPublisherSubjects  = []string{"com.hollow.sh.controllers.replies"}
	defaultNatsStreamSubscriberSubjects = []string{"com.hollow.sh.controllers.commands.>"}
	defaultNatsConnectTimeout           = 100 * time.Millisecond
)

// NATs streaming configuration

// TODO: move project to use viper instead and use viper methods here.
func (c *Config) LoadNatsEnvVars() error {
	c.setNATSDefaults()

	if natsURL := os.Getenv("ALLOY_NATS_URL"); natsURL != "" {
		c.NatsOptions.StreamURL = natsURL
	}

	if c.NatsOptions.StreamURL == "" {
		return errors.New("missing parameter: nats.url")
	}

	if natsStreamUser := os.Getenv("ALLOY_NATS_STREAM_USER"); natsStreamUser != "" {
		c.NatsOptions.StreamUser = natsStreamUser
	}

	if natsStreamPass := os.Getenv("ALLOY_NATS_STREAM_PASS"); natsStreamPass != "" {
		c.NatsOptions.StreamPass = natsStreamPass
	}

	if natsCredsFile := os.Getenv("ALLOY_NATS_CREDS_FILE"); natsCredsFile != "" {
		c.NatsOptions.CredsFile = natsCredsFile
	}

	if natsStreamName := os.Getenv("ALLOY_NATS_STREAM_NAME"); natsStreamName != "" {
		c.NatsOptions.StreamName = natsStreamName
	}

	if natsStreamPrefix := os.Getenv("ALLOY_NATS_STREAM_PREFIX"); natsStreamPrefix != "" {
		c.NatsOptions.StreamPrefix = natsStreamPrefix
	}

	if natsPublisherSubjects := os.Getenv("ALLOY_NATS_PUBLISHER_SUBJECTS"); natsPublisherSubjects != "" {
		c.NatsOptions.PublisherSubjects = strings.Split(natsPublisherSubjects, ",")
	}

	if natsSubsciberSubjects := os.Getenv("ALLOY_NATS_SUBSCRIBER_SUBJECTS"); natsSubsciberSubjects != "" {
		c.NatsOptions.SubscriberSubjects = strings.Split(natsSubsciberSubjects, ",")
	}

	if natsUrnNS := os.Getenv("ALLOY_NATS_URN_NS"); natsUrnNS != "" {
		c.NatsOptions.StreamURNNamespace = natsUrnNS
	}

	if connectTimeout := os.Getenv("ALLOY_NATS_STREAM_CONNECT_TIMEOUT"); connectTimeout != "" {
		d, err := time.ParseDuration(connectTimeout)
		if err != nil {
			return err
		}

		c.NatsOptions.ConnectTimeout = d
	}

	return nil
}

func (c *Config) setNATSDefaults() {
	if c.NatsOptions.StreamName == "" {
		c.NatsOptions.StreamName = defaultNatsStreamName
	}

	if c.NatsOptions.StreamPrefix == "" {
		c.NatsOptions.StreamPrefix = defaultNatsStreamPrefix
	}

	if c.NatsOptions.StreamURNNamespace == "" {
		c.NatsOptions.StreamURNNamespace = defaultNatsStreamURNNamespace
	}

	if len(c.NatsOptions.PublisherSubjects) == 0 {
		c.NatsOptions.PublisherSubjects = defaultNatsStreamPublisherSubjects
	}

	if len(c.NatsOptions.SubscriberSubjects) == 0 {
		c.NatsOptions.SubscriberSubjects = defaultNatsStreamSubscriberSubjects
	}

	if c.NatsOptions.ConnectTimeout == 0 {
		c.NatsOptions.ConnectTimeout = defaultNatsConnectTimeout
	}
}
