package app

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	ErrAppInit = errors.New("error initializing app")
)

// App holds attributes for running alloy.
type App struct {
	// Viper loads configuration parameters.
	v *viper.Viper
	// App configuration.
	Config *Configuration
	// TermCh is the channel to terminate the app based on a signal
	TermCh chan os.Signal
	// Sync waitgroup to wait for running go routines on termination.
	SyncWg *sync.WaitGroup
	// Logger is the app logger
	Logger *logrus.Logger
}

// New returns a new alloy application object with the configuration loaded
func New(ctx context.Context, kind model.AppKind, cfgFile string, loglevel string) (app *App, err error) {
	switch kind {
	case model.AppKindInband, model.AppKindOutOfBand:
	default:
		return nil, errors.Wrap(ErrAppInit, "invalid app kind: "+string(kind))
	}

	app = &App{
		v:      viper.New(),
		TermCh: make(chan os.Signal),
		SyncWg: &sync.WaitGroup{},
		Logger: logrus.New(),
	}

	if err := app.LoadConfiguration(cfgFile); err != nil {
		return nil, err
	}

	// set here again since LoadConfiguration could overwrite it.
	app.Config.AppKind = kind

	switch model.LogLevel(loglevel) {
	case model.LogLevelDebug:
		app.Logger.Level = logrus.DebugLevel
	case model.LogLevelTrace:
		app.Logger.Level = logrus.TraceLevel
	default:
		app.Logger.Level = logrus.InfoLevel
	}

	app.Logger.SetFormatter(&logrus.JSONFormatter{})

	// register for SIGINT, SIGTERM
	signal.Notify(app.TermCh, syscall.SIGINT, syscall.SIGTERM)

	return app, nil
}

// NewLogrusEntryFromLogger returns a logger contextualized with the given logrus fields.
func NewLogrusEntryFromLogger(fields logrus.Fields, logger *logrus.Logger) *logrus.Entry {
	l := logrus.New()
	l.Formatter = logger.Formatter
	loggerEntry := logger.WithFields(fields)
	loggerEntry.Level = logger.Level

	return loggerEntry
}
