package asset

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/gammazero/workerpool"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// default number of concurrent requests
	concurrency = 5
	// delay between EMAPI requests
	delayBetweenRequests = 2 * time.Second
	// SourceKindEMAPI identifies an Equinic metal API asset getter.
	SourceKindEMAPI = "emapi"
)

var (
	ErrEMAPI = errors.New("emapi error")

	// default count of assets to retrieve per request
	batchSize = 5
)

// emapi is an asset sourcer
type emapi struct {
	client  emapiRequestor
	logger  *logrus.Entry
	config  *model.Config
	syncWg  *sync.WaitGroup
	assetCh chan<- *model.Asset
	workers *workerpool.WorkerPool
}

// NewEMAPISource returns a new emapi asset sourcer to retieve asset information from EMAPI for inventory collection.
func NewEMAPISource(ctx context.Context, alloy *app.App) (Getter, error) {
	client, err := newEMAPIClient(alloy.Config, alloy.Logger)
	if err != nil {
		return nil, err
	}

	e := &emapi{
		logger:  alloy.Logger.WithField("component", "getter.emapi"),
		syncWg:  alloy.SyncWg,
		config:  alloy.Config,
		assetCh: alloy.AssetCh,
		client:  client,
	}

	e.workers = workerpool.New(alloy.Config.AssetGetter.Emapi.Concurrency)

	return e, nil
}

func (e *emapi) SetClient(client interface{}) {
	e.client = client.(emapiRequestor)
}

func (e *emapi) ListByIDs(ctx context.Context, assetIDs []string) error {
	// close assetCh to notify consumers
	defer close(e.assetCh)

	// channel for routines spawned to indicate completion
	doneCh := make(chan struct{})

	// count of routines spawned to retrieve assets
	var dispatched int32

	// submit inventory collection to worker pool
	for _, assetID := range assetIDs {
		assetID := assetID

		// increment wait group
		e.syncWg.Add(1)

		// increment spawned count
		atomic.AddInt32(&dispatched, 1)

		e.workers.Submit(
			func() {
				defer e.syncWg.Done()
				defer func() { doneCh <- struct{}{} }()

				// lookup asset by its ID from the inventory asset store
				asset, err := e.client.AssetByID(ctx, assetID)
				if err != nil {
					e.logger.Warn(err)
				}

				// send asset for inventory collection
				e.assetCh <- asset
			},
		)
	}

	for dispatched > 0 {
		<-doneCh
		atomic.AddInt32(&dispatched, ^int32(0))
	}

	return nil
}

func (e *emapi) ListAll(ctx context.Context) error {
	// close assetCh to notify consumers
	defer close(e.assetCh)

	// channel for routines spawned to indicate completion
	doneCh := make(chan struct{})

	// count of routines spawned to retrieve assets
	var dispatched int32

	// increment wait group
	e.syncWg.Add(1)

	// increment spawned count
	atomic.AddInt32(&dispatched, 1)

	func(dispatched *int32) {
		e.workers.Submit(

			func() {
				defer e.syncWg.Done()
				defer func() { doneCh <- struct{}{} }()

				err := e.dispatcher(ctx, dispatched, doneCh)
				if err != nil {
					e.logger.Warn(err)
				}
			},
		)
	}(&dispatched)

	for dispatched > 0 {
		<-doneCh
		atomic.AddInt32(&dispatched, ^int32(0))
	}

	return nil
}

// dispatcher queries EMAPI for total assets and spawns workers to retrieve asset information
//
// nolint:gocyclo // this method has various cases to consider and shared context information which is ideal to keep together.
func (e *emapi) dispatcher(ctx context.Context, dispatched *int32, doneCh chan<- struct{}) error {
	// first request to figures out total items
	beginOffset := 1

	assets, total, err := e.client.AssetsByOffsetLimit(ctx, beginOffset, 1)
	if err != nil {
		return err
	}

	// submit the assets collected in the first request
	for _, asset := range assets {
		e.assetCh <- asset
	}

	if total == 1 {
		return nil
	}

	var finalBatch bool

	// continue from offset 2
	beginOffset = 2
	for idx := beginOffset; idx <= total; {
		// final batch
		if total > batchSize && total-beginOffset <= batchSize {
			batchSize = total - beginOffset
			finalBatch = true
		}

		for e.workers.WaitingQueueSize() > 0 {
			if ctx.Err() != nil {
				break
			}

			e.logger.WithFields(logrus.Fields{
				"component":   "asset getter",
				"queue size":  e.workers.WaitingQueueSize(),
				"concurrency": e.config.AssetGetter.Emapi.Concurrency,
			}).Debug("delay for queue size to drop..")

			// nolint:gomnd // delay is a magic number
			time.Sleep(5 * time.Second)
		}

		// increment wait group
		e.syncWg.Add(1)

		// increment spawned count
		atomic.AddInt32(dispatched, 1)

		// pause between spawning workers - skip delay for tests
		if os.Getenv("TEST_ENV") == "" {
			time.Sleep(delayBetweenRequests)
		}

		// spawn worker with the offset, limit parameters
		// this is done within a closure to capture the offset, limit values
		func(offset, limit int) {
			e.workers.Submit(
				func() {
					defer e.syncWg.Done()
					defer func() { doneCh <- struct{}{} }()

					e.logger.WithFields(logrus.Fields{
						"offset": offset,
						"limit":  limit,
					}).Trace()

					assets, _, err := e.client.AssetsByOffsetLimit(ctx, offset, limit)
					if err != nil {
						e.logger.Warn(err)
					}

					for _, asset := range assets {
						e.assetCh <- asset
					}
				},
			)
		}(beginOffset, batchSize)

		if !finalBatch {
			idx += batchSize
			beginOffset = idx
		} else {
			break
		}
	}

	return nil
}

type emapiRequestor interface {
	// AssetByID queries EMAPI for an asset and returns its model.Asset equivalent object.
	AssetByID(ctx context.Context, id string) (asset *model.Asset, err error)
	AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, total int, err error)
}

type emapiClient struct {
	client        *retryablehttp.Client
	endpoint      *url.URL
	customHeaders map[string]string
	logger        *logrus.Logger
	authToken     string
	consumerToken string
	facility      string
}

// newEMAPIClient validates the EMAPI configuration object and returns a emapiClient object
//
// nolint:gocyclo // validation is cyclomatic
func newEMAPIClient(cfg *model.Config, logger *logrus.Logger) (*emapiClient, error) {
	if cfg == nil {
		return nil, errors.Wrap(ErrConfig, "expected valid Config object, got nil")
	}

	// env var auth token
	if authToken := os.Getenv("EMAPI_AUTH_TOKEN"); authToken != "" {
		cfg.AssetGetter.Emapi.AuthToken = authToken
	}

	if cfg.AssetGetter.Emapi.AuthToken == "" {
		return nil, errors.Wrap(ErrConfig, "expected valid emapi auth token, got empty")
	}

	// env var consumer token
	if consumerToken := os.Getenv("EMAPI_CONSUMER_TOKEN"); consumerToken != "" {
		cfg.AssetGetter.Emapi.ConsumerToken = consumerToken
	}

	if cfg.AssetGetter.Emapi.ConsumerToken == "" {
		return nil, errors.Wrap(ErrConfig, "expected valid emapi consumer token, got empty")
	}

	if cfg.AssetGetter.Emapi.Facility == "" {
		return nil, errors.Wrap(ErrConfig, "expected valid emapi facility, got empty")
	}

	// env var endpoint
	if endpoint := os.Getenv("EMAPI_ENDPOINT"); endpoint != "" {
		cfg.AssetGetter.Emapi.Endpoint = endpoint
	}

	endpoint, err := url.Parse(cfg.AssetGetter.Emapi.Endpoint)
	if err != nil {
		return nil, errors.Wrap(ErrConfig, "error in emapi endpoint URL: "+err.Error())
	}

	if cfg.AssetGetter.Emapi.BatchSize == 0 {
		cfg.AssetGetter.Emapi.BatchSize = batchSize
	}

	if cfg.AssetGetter.Emapi.Concurrency == 0 {
		cfg.AssetGetter.Emapi.Concurrency = concurrency
	}

	// init retryable http client
	retryableClient := retryablehttp.NewClient()

	// disable default debug logging on the retryable client
	if logger.Level < logrus.DebugLevel {
		retryableClient.Logger = nil
	} else {
		retryableClient.Logger = logger
	}

	return &emapiClient{
		client:        retryableClient,
		endpoint:      endpoint,
		authToken:     cfg.AssetGetter.Emapi.AuthToken,
		consumerToken: cfg.AssetGetter.Emapi.ConsumerToken,
		facility:      cfg.AssetGetter.Emapi.Facility,
		customHeaders: cfg.AssetGetter.Emapi.CustomHeaders,
		logger:        logger,
	}, nil
}

// assetByID queries emapi for the hardware asset by ID and returns an Asset object
func (c *emapiClient) AssetByID(ctx context.Context, id string) (*model.Asset, error) {
	hardware, err := c.requestHardwareByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(ErrEMAPI, err.Error())
	}

	// ensure hardware object has all required attributes
	switch {
	case hardware.ID == "":
		return nil, errors.Wrap(ErrEMAPI, "queried hardware object has no ID attribute set")
	case hardware.BMC.Address == "":
		return nil, errors.Wrap(ErrEMAPI, "queried hardware object has no BMC address attribute set")
	case hardware.BMC.Username == "":
		return nil, errors.Wrap(ErrEMAPI, "queried hardware object has no BMC username attribute set")
	case hardware.BMC.Password == "":
		return nil, errors.Wrap(ErrEMAPI, "queried hardware object has no BMC password attribute set")
	}

	return c.toAsset(hardware), nil
}

func (c *emapiClient) toAsset(h *Hardware) *model.Asset {
	return &model.Asset{
		ID:          h.ID,
		Model:       h.Model,
		Vendor:      h.Manufacturer.Name,
		BMCUsername: h.BMC.Username,
		BMCPassword: h.BMC.Password,
		BMCAddress:  net.ParseIP(h.BMC.Address),
	}
}

// APIData struct is the response from the Packet API
type APIData struct {
	APIMeta struct {
		CurrentPage *int `json:"current_page"`
		LastPage    *int `json:"next_page"`
		PrevPage    *int `json:"prev_page"`
		TotalPages  *int `json:"total_pages"`
	} `json:"meta"`
	Hardware []Hardware `json:"hardware"`
}

// Hardware struct is where hardware data is unmashalled into
type Hardware struct {
	ID           string `json:"id"`
	Model        string `json:"model_number"`
	Manufacturer struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"manufacturer"`

	BMC struct {
		Address  string `json:"address"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"management"`
}

func (c *emapiClient) requestHardwareByID(ctx context.Context, id string) (*Hardware, error) {
	endpoint := *c.endpoint

	query := endpoint.Query()
	query.Add("include", "manufacturer")

	endpoint.RawQuery = query.Encode()
	endpoint.Path += "/hardware/" + id

	body, statusCode, err := c.query(ctx, endpoint.String(), "GET", nil)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"url":        endpoint,
				"err":        err,
				"statusCode": statusCode,
			}).Error("error returned in EMAPI request")

		return nil, err
	}

	hardware := &Hardware{}

	err = json.Unmarshal(body, hardware)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"url": endpoint,
				"err": err,
			}).Error("error in EMAPI response unmarshal")

		return nil, err
	}

	return hardware, nil
}

func (c *emapiClient) AssetsByOffsetLimit(ctx context.Context, offset, limit int) (assets []*model.Asset, total int, err error) {
	endpoint := *c.endpoint

	query := endpoint.Query()
	query.Add("include", "manufacturer")
	query.Add("facility", c.facility)
	query.Add("type", "server")
	query.Add("model", "r6515")
	query.Add("page", strconv.Itoa(offset))
	query.Add("per_page", strconv.Itoa(limit))

	endpoint.RawQuery = query.Encode()
	endpoint.Path += "/hardware/"

	body, statusCode, err := c.query(ctx, endpoint.String(), "GET", nil)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"url":        endpoint.String(),
				"err":        err,
				"statusCode": statusCode,
			}).Error("error returned in EMAPI request")

		return nil, 0, err
	}

	data := &APIData{}

	err = json.Unmarshal(body, data)
	if err != nil {
		c.logger.WithFields(
			logrus.Fields{
				"url": endpoint,
				"err": err,
			}).Error("error in EMAPI response unmarshal")

		return nil, 0, err
	}

	assets = make([]*model.Asset, 0, len(data.Hardware))

	for _, hw := range data.Hardware {
		hw := hw
		assets = append(assets, c.toAsset(&hw))
	}

	return assets, *data.APIMeta.TotalPages, nil
}

func (c *emapiClient) query(ctx context.Context, endpoint, method string, payload []byte) (body []byte, statusCode int, err error) {
	var req *http.Request

	if len(payload) > 0 {
		req, err = http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, endpoint, http.NoBody)
	}

	if err != nil {
		return body, 0, err
	}

	req.Header.Add("X-Auth-Token", c.authToken)
	req.Header.Add("X-Consumer-Token", c.consumerToken)
	req.Header.Add("Content-Type", "application/json")

	for key, value := range c.customHeaders {
		req.Header.Add(key, value)
	}

	requestRetryable, err := retryablehttp.FromRequest(req)
	if err != nil {
		return body, 0, err
	}

	resp, err := c.client.Do(requestRetryable)
	if err != nil {
		return body, 0, err
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return body, 0, err
	}

	defer resp.Body.Close()

	return body, resp.StatusCode, nil
}
