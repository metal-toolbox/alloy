package outofband

import (
	"context"
	"os"
	"strings"
	"time"

	logrusr "github.com/bombsimon/logrusr/v4"
	"github.com/jacobweinstock/registrar"
	common "github.com/metal-toolbox/bmc-common"
	bmclib "github.com/metal-toolbox/bmclib"
	"github.com/pkg/errors"
	"github.com/sanity-io/litter"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
)

var (
	ErrInventory  = errors.New("inventory collection error")
	ErrBiosConfig = errors.New("BIOS configuration collection error")
	ErrConnect    = errors.New("BMC connection error")
	ErrBMCSession = errors.New("BMC session error")
)

const (
	// logoutTimeout is the timeout value for each bmc logout attempt.
	logoutTimeout = "1m"

	// bmclib will attempt multiple providers (drivers) - to perform an action,
	// this is maximum amount of time bmclib will spend performing a query on a BMC.
	bmclibProviderTimeout = 180 * time.Second

	pkgName = "internal/outofband"

	LoginError         model.CollectorError = "LoginError"
	InventoryError     model.CollectorError = "InventoryError"
	GetBiosConfigError model.CollectorError = "GetBiosConfigError"
)

// OutOfBand collector collects hardware, firmware inventory out of band
type Queryor struct {
	mockClient    BMCQueryor
	logger        *logrus.Entry
	logoutTimeout time.Duration
}

// BMCQueryor interface defines methods that the bmclib client exposes
// this is mainly to swap the bmclib instance for tests
type BMCQueryor interface {
	Open(ctx context.Context) error
	Close(ctx context.Context) error
	Inventory(ctx context.Context) (*common.Device, error)
	GetBiosConfiguration(ctx context.Context) (map[string]string, error)
	GetPowerState(ctx context.Context) (state string, err error)
}

// NewQueryor returns a instance of the Queryor inventory collector
func NewQueryor(logger *logrus.Logger) *Queryor {
	lt, err := time.ParseDuration(logoutTimeout)
	if err != nil {
		panic(err)
	}

	c := &Queryor{
		logger:        logger.WithFields(logrus.Fields{"component": "collector.outofband"}),
		logoutTimeout: lt,
	}

	return c
}

// Inventory retrieves device component and firmware information
// and updates the given asset object with the inventory
func (o *Queryor) Inventory(ctx context.Context, asset *model.Asset) error {
	// attach child span
	ctx, span := otel.Tracer(pkgName).Start(ctx, "Inventory")
	defer span.End()

	setTraceSpanAssetAttributes(span, asset)

	o.logger.WithFields(
		logrus.Fields{
			"serverID": asset.ID,
			"IP":       asset.BMCAddress.String(),
		}).Trace("logging into to BMC")

	// login
	bmc, err := o.bmcLogin(ctx, asset)
	if err != nil {
		return err
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
	return o.bmcInventory(ctx, bmc, asset)
}

func (o *Queryor) BiosConfiguration(ctx context.Context, asset *model.Asset) error {
	// attach child span
	ctx, span := otel.Tracer(pkgName).Start(ctx, "BiosConfiguration")
	defer span.End()

	setTraceSpanAssetAttributes(span, asset)

	// login
	bmc, err := o.bmcLogin(ctx, asset)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"serverID": asset.ID,
				"IP":       asset.BMCAddress.String(),
				"err":      err,
			}).Warn("BMC login error")

		return err
	}

	// defer logout
	//
	// ctx is not passed to bmcLogout to ensure that
	// the bmc logout is carried out even if the context is canceled.
	defer o.bmcLogout(bmc, asset)

	// collect bios configuration
	return o.biosConfiguration(ctx, bmc, asset)
}

// biosConfiguration collects bios configuration data from the BMC
// it updates the asset.BiosConfig attribute with the data collected.
//
// If any errors occurred in the collection, those are included in the asset.Errors attribute.
func (o *Queryor) biosConfiguration(ctx context.Context, bmc BMCQueryor, asset *model.Asset) error {
	// measure BMC biosConfiguration query
	startTS := time.Now()

	biosConfig, err := bmc.GetBiosConfiguration(ctx)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"serverID": asset.ID,
				"IP":       asset.BMCAddress.String(),
				"err":      err,
			}).Warn("error in bmc bios configuration collection")

		trace.SpanFromContext(ctx).SetStatus(codes.Error, " BMC GetBiosConfiguration(): "+err.Error())

		// increment get bios configuration query error count metric
		switch {
		case strings.Contains(err.Error(), "no compatible System Odata IDs identified"):
			// device provides a redfish API, but BIOS configuration export isn't supported in the current redfish library
			asset.AppendError(GetBiosConfigError, "redfish_incompatible: no compatible System Odata IDs identified")
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "redfish_incompatible")
		case strings.Contains(err.Error(), "no BiosConfigurationGetter implementations found"):
			// no means to export BIOS configuration were found
			asset.AppendError(GetBiosConfigError, "device not supported")
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "NoBiosConfigurationGetter")
		default:
			asset.AppendError(GetBiosConfigError, err.Error())
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "GetBiosConfigurationError")
		}

		return errors.Wrap(ErrBiosConfig, err.Error())
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
func (o *Queryor) bmcInventory(ctx context.Context, bmc BMCQueryor, asset *model.Asset) error {
	// measure BMC inventory query
	startTS := time.Now()

	inventory, err := bmc.Inventory(ctx)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"serverID": asset.ID,
				"IP":       asset.BMCAddress.String(),
				"err":      err,
			}).Warn("error in bmc inventory collection")

		trace.SpanFromContext(ctx).SetStatus(codes.Error, " BMC Inventory(): "+err.Error())

		// increment inventory query error count metric
		if strings.Contains(err.Error(), "no compatible System Odata IDs identified") {
			// device provides a redfish API, but inventory export isn't supported in the current redfish library
			asset.AppendError(InventoryError, "redfish_incompatible: no compatible System Odata IDs identified")
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "redfish_incompatible")
		} else {
			asset.AppendError(InventoryError, err.Error())
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "inventory")
		}

		return errors.Wrap(ErrInventory, err.Error())
	}

	if inventory == nil {
		return errors.Wrap(ErrInventory, "nil *common.Device object returned")
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
func (o *Queryor) bmcLogin(ctx context.Context, asset *model.Asset) (BMCQueryor, error) {
	// bmc is the bmc client instance
	var bmc BMCQueryor

	// attach child span
	ctx, span := otel.Tracer(pkgName).Start(ctx, "bmcLogin")
	defer span.End()

	if o.mockClient == nil {
		bmc = newBMCClient(
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

		switch {
		case strings.Contains(err.Error(), "operation timed out"):
			asset.AppendError(LoginError, "operation timed out in "+time.Since(startTS).String())
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "conn_timeout")
		case strings.Contains(err.Error(), "401: "), strings.Contains(err.Error(), "failed to login"):
			asset.AppendError(LoginError, "unauthorized")
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "unauthorized")
		default:
			asset.AppendError(LoginError, err.Error())
			metrics.IncrementBMCQueryErrorCount(asset.Vendor, asset.Model, "other")
		}

		return nil, errors.Wrap(ErrConnect, err.Error())
	}

	// measure BMC connection open query time
	metrics.ObserveBMCQueryTimeSummary(asset.Vendor, asset.Model, "conn_open", startTS)

	return bmc, nil
}

func (o *Queryor) bmcLogout(bmc BMCQueryor, asset *model.Asset) {
	// measure BMC connection close
	startTS := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), o.logoutTimeout)
	defer cancel()

	ctx, span := otel.Tracer(pkgName).Start(ctx, "bmclibLogOut")
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
func newBMCClient(asset *model.Asset, l *logrus.Logger) *bmclib.Client {
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

	logruslogr := logrusr.New(logger)

	bmcClient := bmclib.NewClient(
		asset.BMCAddress.String(),
		asset.BMCUsername,
		asset.BMCPassword,
		bmclib.WithLogger(logruslogr),
		bmclib.WithPerProviderTimeout(bmclibProviderTimeout),
	)

	// set bmclib driver
	//
	// The bmclib drivers here are limited to the HTTPS means of connection,
	// that is, drivers like ipmi are excluded.
	switch asset.Vendor {
	case common.VendorDell, common.VendorHPE:
		// Set to the bmclib ProviderProtocol value
		// https://github.com/metal-toolbox/bmclib/blob/providers/redfish/redfish.go#L26
		bmcClient.Registry.Drivers = bmcClient.Registry.Using("redfish")
	case common.VendorAsrockrack:
		// https://github.com/metal-toolbox/bmclib/blob/providers/asrockrack/asrockrack.go#L20
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

func (o *Queryor) SessionActive(ctx context.Context, bmc BMCQueryor) bool {
	if bmc == nil {
		return false
	}

	// check if we're able to query the power state
	powerStatus, err := bmc.GetPowerState(ctx)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"err": err.Error(),
			},
		).Trace("session not active, checked with GetPowerState()")

		return false
	}

	o.logger.WithFields(
		logrus.Fields{
			"powerStatus": powerStatus,
		},
	).Trace("session currently active, checked with GetPowerState()")

	return true
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
