package csv

import (
	"context"
	"encoding/csv"
	"io"
	"net"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// SourceKindCSV identifies a csv asset source
	SourceKindCSV = "csv"
)

var (
	ErrAssetNotFound = errors.New("not found")
	ErrCSVSource     = errors.New("error in CSV")
)

type Store struct {
	csvReader io.ReadCloser
	logger    *logrus.Entry
}

// New returns a new csv asset getter to retrieve asset information from a CSV file for inventory collection.
func New(_ context.Context, csvFile string, logger *logrus.Logger) (*Store, error) {
	fh, err := os.Open(csvFile)
	if err != nil {
		return nil, err
	}

	return &Store{
		logger:    logger.WithField("component", "store.csv"),
		csvReader: fh,
	}, nil
}

// Kind returns the repository store kind.
func (c *Store) Kind() model.StoreKind {
	return model.StoreKindCsv
}

// AssetByID returns one asset from the inventory identified by its identifier.
func (c *Store) AssetByID(ctx context.Context, assetID string, _ bool) (*model.Asset, error) {
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

func (c *Store) AssetsByOffsetLimit(_ context.Context, _, _ int) (assets []*model.Asset, totalAssets int, err error) {
	return nil, 0, nil
}

func (c *Store) AssetUpdate(_ context.Context, _ *model.Asset) error {
	return nil
}

// loadAssets returns a slice of assets from the given csv io.Reader
func (c *Store) loadAssets(_ context.Context, csvReader io.ReadCloser) ([]*model.Asset, error) {
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
