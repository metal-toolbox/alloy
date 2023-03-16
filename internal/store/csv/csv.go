package store

import (
	"context"
	"encoding/csv"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// SourceKindCSV identifies a csv asset source
	SourceKindCSV = "csv"
)

var (
	ErrCSVSource = errors.New("error in CSV")
)

type csvee struct {
	csvReader io.ReadCloser
	logger    *logrus.Entry
	config    *app.Configuration
	syncWg    *sync.WaitGroup
	assetCh   chan<- *model.Asset
}

// NewCSVGetter returns a new csv asset getter to retrieve asset information from a CSV file for inventory collection.
func NewCSVGetter(ctx context.Context, alloy *app.App, csvReader io.ReadCloser) (*csvee, error) {
	return &csvee{
		logger:    alloy.Logger.WithField("component", "getter-csv"),
		syncWg:    alloy.SyncWg,
		config:    alloy.Config,
		csvReader: csvReader,
	}, nil
}

// SetClient satisfies the Getter interface
func (c *csvee) SetClient(client interface{}) {
}

// AssetByID returns one asset from the inventory identified by its identifier.
func (c *csvee) AssetByID(ctx context.Context, assetID string, fetchBmcCredentials bool) (*model.Asset, error) {
	// attach child span
	ctx, span := tracer.Start(ctx, "AssetByID()")
	defer span.End()

	assets, err := c.loadAssets(ctx, c.csvReader)
	if err != nil {
		return nil, err
	}

	for _, asset := range assets {
		if asset.ID == assetID {
			return asset, nil
		}
	}

	return nil, errors.Wrap(ErrAssetNotFound, assetID)
}

// ListAll runs the csv asset getter which returns all assets on the assetCh
func (c *csvee) ListAll(ctx context.Context) error {
	// channel close tells the channel reader we're done
	defer close(c.assetCh)

	c.logger.Trace("load assets")

	assets, err := c.loadAssets(ctx, c.csvReader)
	if err != nil {
		return err
	}

	c.logger.WithField("count", len(assets)).Trace("loaded assets")

	for _, asset := range assets {
		c.assetCh <- asset
	}

	return nil
}

// ListByIDs runs the csv asset getter which returns  on the assetCh
func (c *csvee) ListByIDs(ctx context.Context, assetIDs []string) error {
	// channel close tells the channel reader we're done
	defer close(c.assetCh)

	assets, err := c.loadAssets(ctx, c.csvReader)
	if err != nil {
		return err
	}

	for _, asset := range assets {
		if sliceContains(assetIDs, asset.ID) {
			c.assetCh <- asset
		}
	}

	return nil
}

func (c *csvee) AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, totalAssets int, err error) {
	return nil, 0, nil
}

func sliceContains(sl []string, str string) bool {
	for _, elem := range sl {
		if elem == str {
			return true
		}
	}

	return false
}

// loadAssets returns a slice of assets from the given csv io.Reader
func (c *csvee) loadAssets(ctx context.Context, csvReader io.ReadCloser) ([]*model.Asset, error) {
	records, err := csv.NewReader(csvReader).ReadAll()
	if err != nil {
		return nil, err
	}

	defer csvReader.Close()

	if len(records) == 0 {
		return nil, errors.Wrap(ErrCSVSource, "no valid asset records found")
	}

	assets := []*model.Asset{}

	// csv is of the format
	// uuid, IP address, username, password
	for idx, rec := range records {
		// skip csv header
		if idx == 0 {
			continue
		}

		id := strings.TrimSpace(rec[0])
		ip := strings.TrimSpace(rec[1])
		username := strings.TrimSpace(rec[2])
		password := strings.TrimSpace(rec[3])

		var vendor string

		// nolint:gomnd // field 4 is the vendor name, and its optional.
		if len(rec) > 4 {
			vendor = strings.TrimSpace(rec[4])
		}

		_, err := uuid.Parse(strings.TrimSpace(id))
		if err != nil {
			return nil, errors.Wrap(ErrCSVSource, err.Error()+": "+id)
		}

		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			return nil, errors.Wrap(ErrCSVSource, "invalid IP address: "+ip)
		}

		if username == "" {
			return nil, errors.Wrap(ErrCSVSource, "invalid username string")
		}

		if password == "" {
			return nil, errors.Wrap(ErrCSVSource, "invalid password string")
		}

		assets = append(
			assets,
			&model.Asset{
				ID:          id,
				BMCUsername: username,
				BMCPassword: password,
				BMCAddress:  parsedIP,
				Vendor:      vendor,
			},
		)
	}

	return assets, nil
}
