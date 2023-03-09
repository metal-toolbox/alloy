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

// SetAssetChannel sets/overrides the asset channel on the publisher
func (p *stdoutPublisher) SetAssetChannel(assetCh chan *model.Asset) {
	p.collectorCh = assetCh
}

// RunInventoryPublisher iterates over device objects received on the collector channel and prints them to stdout,
// the method stops once the collector channel is closed.
//
// RunInventoryPublisher implements the Publisher interface
func (p *stdoutPublisher) RunInventoryPublisher(ctx context.Context) error {
	for device := range p.collectorCh {
		if device == nil {
			continue
		}

		out, err := json.MarshalIndent(device, "", " ")
		if err != nil {
			return err
		}

		log.Println(string(out))
	}

	return nil
}

// PublishOne publishes the device parameter by printing the device object to stdout.
//
// PublishOne implements the Publisher interface
func (p *stdoutPublisher) PublishOne(ctx context.Context, device *model.Asset) error {
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
