package outofband

import (
	"context"
	"os"
	"strings"
	"time"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/common"
	logrusrv2 "github.com/bombsimon/logrusr/v2"
	"github.com/jacobweinstock/registrar"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/pkg/errors"
	"github.com/sanity-io/litter"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	// The outofband collector tracer
	tracer     trace.Tracer
	ErrConnect = errors.New("error connecting to BMC")
)

func init() {
	tracer = otel.Tracer("collector-outofband")
}

const (
	// concurrency is the default number of workers to concurrently query BMCs
	concurrency = 20

	// logoutTimeout is the timeout value for each bmc logout attempt.
	logoutTimeout = "1m"
)

// OutOfBand collector collects hardware, firmware inventory out of band
type Collector struct {
	mockClient    oobGetter
	logger        *logrus.Entry
	logoutTimeout time.Duration
}

// oobGetter interface defines methods that the bmclib client exposes
// this is mainly to swap the bmclib instance for tests
type oobGetter interface {
	Open(ctx context.Context) error
	Close(ctx context.Context) error
	Inventory(ctx context.Context) (*common.Device, error)
	GetBiosConfiguration(ctx context.Context) (map[string]string, error)
}

// NewCollector returns a instance of the Collector inventory collector
func NewCollector(logger *logrus.Logger) *Collector {
	loggerEntry := app.NewLogrusEntryFromLogger(
		logrus.Fields{"component": "collector.outofband"},
		logger,
	)

	lt, err := time.ParseDuration(logoutTimeout)
	if err != nil {
		panic(err)
	}

	c := &Collector{
		logger:        loggerEntry,
		logoutTimeout: lt,
	}

	return c
}

// CollectForAsset runs the asset inventory, bios data collection and updates the given asset with
// the collected data.
func (o *Collector) CollectForAsset(ctx context.Context, asset *model.Asset) {
	// attach child span
	ctx, span := tracer.Start(ctx, "collect()")
	defer span.End()

	// include asset attributes in trace span
	setTraceSpanAssetAttributes(span, asset)

	o.logger.WithFields(
		logrus.Fields{
			"serverID": asset.ID,
			"IP":       asset.BMCAddress.String(),
		}).Trace("login to BMC..")

	// login
	bmc, err := o.bmcLogin(ctx, asset)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"serverID": asset.ID,
				"IP":       asset.BMCAddress.String(),
				"err":      err,
			}).Warn("BMC login error")

		return
	}

	// defer logout
	//
	// ctx is not passed to bmcLogout to ensure that
	// the bmc logout is carried out even if the context is canceled.
	defer o.bmcLogout(bmc, asset)

	o.logger.WithFields(
		logrus.Fields{
			"serverID": asset.ID,
			"IP":       asset.BMCAddress.String(),
		}).Trace("collecting inventory from asset BMC..")

	// collect inventory
	err = o.bmcInventory(ctx, bmc, asset)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"serverID": asset.ID,
				"IP":       asset.BMCAddress.String(),
				"err":      err,
			}).Warn("BMC inventory error")
	}

	// collect bios configuration
	err = o.bmcGetBiosConfiguration(ctx, bmc, asset)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"serverID": asset.ID,
				"IP":       asset.BMCAddress.String(),
				"err":      err,
			}).Warn("BMC bios configuration collection error")
	}

	return
}

// bmcGetBiosConfiguration collects bios configuration data from the BMC
// it updates the asset.BiosConfig attribute with the data collected.
//
// If any errors occurred in the collection, those are included in the asset.Errors attribute.
func (o *Collector) bmcGetBiosConfiguration(ctx context.Context, bmc oobGetter, asset *model.Asset) error {
	// measure BMC GetBiosConfiguration query
	startTS := time.Now()

	// attach child span
	ctx, span := tracer.Start(ctx, "GetBiosConfiguration()")
	defer span.End()

	biosConfig, err := bmc.GetBiosConfiguration(ctx)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"serverID": asset.ID,
				"IP":       asset.BMCAddress.String(),
				"err":      err,
			}).Warn("error in bmc bios configuration collection")

		span.SetStatus(codes.Error, " BMC GetBiosConfiguration(): "+err.Error())

		// increment get bios configuration query error count metric
		switch {
		case strings.Contains(err.Error(), "no compatible System Odata IDs identified"):
			asset.IncludeError("GetBiosConfigurationError", "redfish_incompatible: no compatible System Odata IDs identified")
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "redfish_incompatible")
		case strings.Contains(err.Error(), "no BiosConfigurationGetter implementations found"):
			// If the asset doesn't implement a BiosConfigurationGetter, skip asset.IncludeError() which allows
			// inventory to be published later on
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "NoBiosConfigurationGetter")
		default:
			asset.IncludeError("GetBiosConfigurationError", err.Error())
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "GetBiosConfigurationError")
		}

		return err
	}

	// measure BMC GetBiosConfiguration query time
	metrics.ObserveBMCQueryTimeSummary(asset.Vendor, asset.Model, "GetBiosConfiguration", startTS)

	asset.BiosConfig = biosConfig

	return nil
}

// bmcInventory collects inventory data from the BMC
// it updates the asset.Inventory attribute with the data collected.
//
// If any errors occurred in the collection, those are included in the asset.Errors attribute.
func (o *Collector) bmcInventory(ctx context.Context, bmc oobGetter, asset *model.Asset) error {
	// measure BMC inventory query
	startTS := time.Now()

	// attach child span
	ctx, span := tracer.Start(ctx, "inventory()")
	defer span.End()

	inventory, err := bmc.Inventory(ctx)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"serverID": asset.ID,
				"IP":       asset.BMCAddress.String(),
				"err":      err,
			}).Warn("error in bmc inventory collection")

		span.SetStatus(codes.Error, " BMC Inventory(): "+err.Error())

		// increment inventory query error count metric
		if strings.Contains(err.Error(), "no compatible System Odata IDs identified") {
			asset.IncludeError("inventory_error", "redfish_incompatible: no compatible System Odata IDs identified")
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "redfish_incompatible")
		} else {
			asset.IncludeError("inventory_error", err.Error())
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "inventory")
		}

		return err
	}

	// measure BMC inventory query time
	metrics.ObserveBMCQueryTimeSummary(asset.Vendor, asset.Model, "inventory", startTS)

	// For debugging and to capture test fixtures data.
	if os.Getenv(model.EnvVarDumpFixtures) == "true" {
		f := asset.ID + ".device.fixture"
		o.logger.Info("oob device fixture dumped as file: ", f)

		// nolint:gomnd // file permissions are clearer in this form.
		_ = os.WriteFile(f, []byte(litter.Sdump(inventory)), 0o600)
	}

	// format the device inventory vendor attribute so its consistent
	inventory.Vendor = common.FormatVendorName(inventory.Vendor)
	asset.Inventory = inventory

	return nil
}

// bmcLogin initiates the BMC session
//
// when theres an error in the login process, asset.Errors is updated to include that information.
func (o *Collector) bmcLogin(ctx context.Context, asset *model.Asset) (oobGetter, error) {
	// bmc is the bmc client instance
	var bmc oobGetter

	// attach child span
	ctx, span := tracer.Start(ctx, "bmcLogin()")
	defer span.End()

	if o.mockClient == nil {
		bmc = newBMCClient(
			ctx,
			asset,
			o.logger.Logger,
		)
	} else {
		// mock client for tests
		bmc = o.mockClient
	}

	// measure BMC connection open
	startTS := time.Now()

	// initiate bmc login session
	if err := bmc.Open(ctx); err != nil {
		span.SetStatus(codes.Error, " BMC login: "+err.Error())

		if strings.Contains(err.Error(), "operation timed out") {
			asset.IncludeError("login_error", "operation timed out in "+time.Since(startTS).String())
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "conn_timeout")
		}

		if strings.Contains(err.Error(), "401: ") || strings.Contains(err.Error(), "failed to login") {
			asset.IncludeError("login_error", "unauthorized")
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "unauthorized")
		}

		return nil, errors.Wrap(ErrConnect, err.Error())
	}

	// measure BMC connection open query time
	metrics.ObserveBMCQueryTimeSummary(asset.Vendor, asset.Model, "conn_open", startTS)

	return bmc, nil
}

func (o *Collector) bmcLogout(bmc oobGetter, asset *model.Asset) {
	// measure BMC connection close
	startTS := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), o.logoutTimeout)
	defer cancel()

	// attach child span
	ctx, span := tracer.Start(ctx, "bmcLogout()")
	defer span.End()

	o.logger.WithFields(
		logrus.Fields{
			"serverID": asset.ID,
			"IP":       asset.BMCAddress.String(),
		}).Trace("bmc connection close")

	if err := bmc.Close(ctx); err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"serverID": asset.ID,
				"IP":       asset.BMCAddress.String(),
				"err":      err,
			}).Warn("error in bmc connection close")

		span.SetStatus(codes.Error, " BMC connection close: "+err.Error())

		// increment connection close error count metric
		metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "conn_close")
	}

	// measure BMC connection open query time
	metrics.ObserveBMCQueryTimeSummary(asset.Vendor, asset.Model, "conn_close", startTS)
}

// newBMCClient initializes a bmclib client with the given credentials
func newBMCClient(ctx context.Context, asset *model.Asset, l *logrus.Logger) *bmclibv2.Client {
	logger := logrus.New()
	logger.Formatter = l.Formatter

	// setup a logr logger for bmclib
	// bmclib uses logr, for which the trace logs are logged with log.V(3),
	// this is a hax so the logrusr lib will enable trace logging
	// since any value that is less than (logrus.LogLevel - 4) >= log.V(3) is ignored
	// https://github.com/bombsimon/logrusr/blob/master/logrusr.go#L64
	switch l.GetLevel() {
	case logrus.TraceLevel:
		logger.Level = 7
	case logrus.DebugLevel:
		logger.Level = 5
	}

	logruslogr := logrusrv2.New(logger)

	bmcClient := bmclibv2.NewClient(
		asset.BMCAddress.String(),
		"", // port unset
		asset.BMCUsername,
		asset.BMCPassword,
		bmclibv2.WithLogger(logruslogr),
	)

	// set bmclibv2 driver
	//
	// The bmclib drivers here are limited to the HTTPS means of connection,
	// that is, drivers like ipmi are excluded.
	switch asset.Vendor {
	case common.VendorDell, common.VendorHPE:
		// Set to the bmclib ProviderProtocol value
		// https://github.com/bmc-toolbox/bmclib/blob/v2/providers/redfish/redfish.go#L26
		bmcClient.Registry.Drivers = bmcClient.Registry.Using("redfish")
	case common.VendorAsrockrack:
		// https://github.com/bmc-toolbox/bmclib/blob/v2/providers/asrockrack/asrockrack.go#L20
		bmcClient.Registry.Drivers = bmcClient.Registry.Using("vendorapi")
	default:
		// attempt both drivers when vendor is unknown
		drivers := append(registrar.Drivers{},
			bmcClient.Registry.Using("redfish")...,
		)

		drivers = append(drivers,
			bmcClient.Registry.Using("vendorapi")...,
		)

		bmcClient.Registry.Drivers = drivers
	}

	return bmcClient
}

// setTraceSpanAssetAttributes includes the asset attributes as span attributes
func setTraceSpanAssetAttributes(span trace.Span, asset *model.Asset) {
	// set span attributes
	span.SetAttributes(attribute.String("bmc.host", asset.BMCAddress.String()))

	if asset.Vendor == "" {
		asset.Vendor = "unknown"
	}

	if asset.Model == "" {
		asset.Model = "unknown"
	}

	span.SetAttributes(attribute.String("bmc.vendor", asset.Vendor))
	span.SetAttributes(attribute.String("bmc.model", asset.Model))
	span.SetAttributes(attribute.String("serverID", asset.ID))
}
