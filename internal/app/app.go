package app

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/metal-toolbox/alloy/internal/helpers"
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
	// AssetGetterPause when set will cause the asset getter to pause sending assets
	// on the asset channel until the flag has been cleared.
	AssetGetterPause *helpers.Pauser
	// App configuration.
	Config *Configuration
	// AssetCh is where the asset getter retrieves assets from the asset store for the inventory collector to consume.
	AssetCh chan *model.Asset
	// CollectorCh is where the asset inventory information is written, for the publisher to consume.
	CollectorCh chan *model.Asset
	// TermCh is the channel to terminate the app based on a signal
	TermCh chan os.Signal
	// Sync waitgroup to wait for running go routines on termination.
	SyncWg *sync.WaitGroup
	// Logger is the app logger
	Logger *logrus.Logger
}

// New returns a new alloy application object with the configuration loaded
func New(ctx context.Context, kind, cfgFile string, loglevel int) (app *App, err error) {
	switch kind {
	case model.AppKindInband, model.AppKindOutOfBand:
	default:
		return nil, errors.Wrap(ErrAppInit, "invalid app kind: "+kind)
	}

	app = &App{
		v:                viper.New(),

		AssetGetterPause: helpers.NewPauser(),
		AssetCh:          make(chan *model.Asset),
		CollectorCh:      make(chan *model.Asset),
		TermCh:           make(chan os.Signal),
		SyncWg:           &sync.WaitGroup{},
		Logger:           logrus.New(),
	}

	if err := app.LoadConfiguration(cfgFile); err != nil {
		return nil, err
	}

	// set here again since LoadConfiguration could overwrite it.
	app.Config.AppKind = kind

	switch loglevel {
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

// InitAssetCollectorChannels is a helper method to initialize the asset and collector channels.
func (a *App) InitAssetCollectorChannels() {
	a.AssetCh = make(chan *model.Asset)
	a.CollectorCh = make(chan *model.Asset)
}

// NewLogrusEntryFromLogger returns a logger contextualized with the given logrus fields.
func NewLogrusEntryFromLogger(fields logrus.Fields, logger *logrus.Logger) *logrus.Entry {
	l := logrus.New()
	l.Formatter = logger.Formatter
	loggerEntry := logger.WithFields(fields)
	loggerEntry.Level = logger.Level

	return loggerEntry
}
