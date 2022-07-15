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
	collectorCh <-chan *model.AssetDevice
	termCh      <-chan os.Signal
}

func NewStdoutPublisher(ctx context.Context, alloy *app.App) (Publisher, error) {
	logger := app.NewLogrusEntryFromLogger(logrus.Fields{"component": "publisher.stdout"}, alloy.Logger)

	p := &stdoutPublisher{
		logger:      logger,
		syncWg:      alloy.SyncWg,
		collectorCh: alloy.CollectorCh,
		termCh:      alloy.TermCh,
	}

	return p, nil
}

func (p *stdoutPublisher) Run(ctx context.Context) error {
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
