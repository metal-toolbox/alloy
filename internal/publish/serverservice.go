package publish

import (
	"context"
	"os"
	"reflect"

	"github.com/google/uuid"
	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/metal-toolbox/alloy/internal/store"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sanity-io/litter"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	r3diff "github.com/r3labs/diff/v3"
	serverservice "go.hollow.sh/serverservice/pkg/api/v1"
)

const (
	KindServerService = "serverService"
)

var (
	// concurrent requests
	concurrency = 5

	ErrSlugs                   = errors.New("slugs error")
	ErrServerServiceQuery      = errors.New("error in server service query")
	ErrRegisterChanges         = errors.New("error in server service register changes")
	ErrAssetObjectConversion   = errors.New("error converting asset object")
	ErrChangeList              = errors.New("error building change list")
	ErrServerServiceAttrObject = errors.New("error in server service attribute object")
	// The serverservice publisher tracer
	tracer trace.Tracer
)

func init() {
	tracer = otel.Tracer("publisher-serverservice")
}

// serverServicePublisher publishes asset inventory to serverService
type serverServicePublisher struct {
	logger               *logrus.Entry
	config               *app.Configuration
	client               *serverservice.Client
	slugs                map[string]*serverservice.ServerComponentType
	firmwares            map[string][]*serverservice.ComponentFirmwareVersion
	attributeNS          string
	versionedAttributeNS string
}

// NewServerServicePublisher returns a serverService publisher to submit inventory data.
func NewServerServicePublisher(ctx context.Context, alloy *app.App) (Publisher, error) {
	logger := app.NewLogrusEntryFromLogger(
		logrus.Fields{"component": "publisher-serverService"},
		alloy.Logger,
	)

	client, err := store.NewServerServiceClient(
		ctx,
		&alloy.Config.ServerserviceOptions,
		logger,
	)
	if err != nil {
		return nil, err
	}

	p := &serverServicePublisher{
		logger:               logger,
		//config:               alloy.Config,
		client:               client,
		slugs:                make(map[string]*serverservice.ServerComponentType),
		firmwares:            make(map[string][]*serverservice.ComponentFirmwareVersion),
		attributeNS:          model.ServerComponentAttributeNS(alloy.Config.AppKind),
		versionedAttributeNS: model.ServerComponentVersionedAttributeNS(alloy.Config.AppKind),
	}

	return p, nil
}

// Publish publishes the given asset to the server service asset store.
//
// Publish implements the Publisher interface.
func (h *serverServicePublisher) Publish(ctx context.Context, device *model.Asset) error {
	if device == nil {
		return nil
	}

	// attach child span
	ctx, span := tracer.Start(ctx, "Publish()")
	defer span.End()

	// cache server component types for lookups
	err := h.cacheServerComponentTypes(ctx)
	if err != nil {
		return err
	}

	// cache server component firmwares for lookups
	err = h.cacheServerComponentFirmwares(ctx)
	if err != nil {
		return err
	}

	h.publish(ctx, device)

	return nil
}

func (h *serverServicePublisher) cacheServerComponentFirmwares(ctx context.Context) error {
	// attach child span
	ctx, span := tracer.Start(ctx, "cacheServerComponentFirmwares()")
	defer span.End()

	// Query ServerService for all firmware
	firmwares, _, err := h.client.ListServerComponentFirmware(ctx, nil)
	if err != nil {
		// count error
		metrics.ServerServiceQueryErrorCount.With(stageLabel).Inc()

		// set span status
		span.SetStatus(codes.Error, "ListServerComponentFirmware() failed")

		return err
	}

	for idx := range firmwares {
		vendor := firmwares[idx].Vendor
		h.firmwares[vendor] = append(h.firmwares[vendor], &firmwares[idx])
	}

	return nil
}
