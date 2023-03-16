package publish

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/sirupsen/logrus"
)

const (
	KindStdout = "stdout"
)

// stdoutPublisher publishes asset inventory to stdout
type stdoutPublisher struct {
	logger      *logrus.Entry
	syncWg      *sync.WaitGroup
	collectorCh <-chan *model.Asset
	termCh      <-chan os.Signal
}

// NewStdoutPublisher returns a new publisher that prints received objects to stdout.
func NewStdoutPublisher(ctx context.Context, alloy *app.App) (Publisher, error) {
	logger := app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher-stdout"}, alloy.Logger)

	p := &stdoutPublisher{
		logger:      logger,
		syncWg:      alloy.SyncWg,
		collectorCh: alloy.CollectorCh,
		termCh:      alloy.TermCh,
	}

	return p, nil
}

// Publish publishes the device parameter by printing the device object to stdout.
//
// Publish implements the Publisher interface
func (p *stdoutPublisher) Publish(ctx context.Context, device *model.Asset) error {
	if device == nil {
		return nil
	}

	out, err := json.MarshalIndent(device, "", " ")
	if err != nil {
		return err
	}

	log.Println(string(out))

	return nil
}
