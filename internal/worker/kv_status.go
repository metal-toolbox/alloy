package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/metal-toolbox/alloy/internal/metrics"
	"github.com/metal-toolbox/alloy/internal/model"
	rkv "github.com/metal-toolbox/rivets/kv"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go.hollow.sh/toolbox/events"
	"go.hollow.sh/toolbox/events/pkg/kv"
)

var (
	inventoryStatusKVBucket = string(model.Inventory)

	defaultKVOpts = []kv.Option{
		kv.WithDescription("Alloy condition status tracking"),
		kv.WithTTL(10 * 24 * time.Hour),
	}
)

func createOrBindKVBucketWithOpts(s events.Stream, opts *events.NatsKVOptions) (nats.KeyValue, error) {
	kvOptions := defaultKVOpts

	if opts.ReplicaCount > 1 {
		kvOptions = append(kvOptions, kv.WithReplicas(opts.ReplicaCount))
	}

	js, ok := s.(*events.NatsJetstream)
	if !ok {
		return nil, errors.New("status KV publisher is only supported on NATS")
	}

	return kv.CreateOrBindKVBucket(js, inventoryStatusKVBucket, kvOptions...)
}

// statusKVPublisher updates the kv with task status information
type statusKVPublisher struct {
	workerID string
	kv       nats.KeyValue
	log      *logrus.Logger
}

func newStatusKVPublisher(s events.Stream, log *logrus.Logger, workerID string, opts *events.NatsKVOptions) (*statusKVPublisher, error) {
	statusKV, err := createOrBindKVBucketWithOpts(s, opts)
	if err != nil {
		return nil, err
	}

	return &statusKVPublisher{
		workerID: workerID,
		kv:       statusKV,
		log:      log,
	}, nil
}

// Publish implements the statemachine Publisher interface.
func (s *statusKVPublisher) Publish(ctx context.Context, task *Task) {
	_, span := otel.Tracer(pkgName).Start(
		ctx,
		"controller.Publish.KV",
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	defer span.End()

	facility := "facility"
	if task.Asset.Facility != "" {
		facility = task.Asset.Facility
	}

	key := fmt.Sprintf("%s.%s", facility, task.ID.String())

	payload := rkv.StatusValue{
		WorkerID: s.workerID,
		Target:   task.Asset.ID,
		TraceID:  trace.SpanFromContext(ctx).SpanContext().TraceID().String(),
		SpanID:   trace.SpanFromContext(ctx).SpanContext().SpanID().String(),
		State:    string(task.State()),
		Status:   statusInfoJSON(task.Status),
		// ResourceVersion:  XXX: the handler context has no concept of this! does this make
		// sense at the controller-level?
		UpdatedAt: time.Now(),
	}

	var err error

	var rev uint64

	if task.Revision == 0 {
		rev, err = s.kv.Create(key, payload.MustBytes())
	} else {
		rev, err = s.kv.Update(key, payload.MustBytes(), task.Revision)
	}

	if err != nil {
		metrics.NATSError("publish-condition-status")
		span.AddEvent("status publish failure",
			trace.WithAttributes(
				attribute.String("workerID", s.workerID),
				attribute.String("serverID", task.Asset.ID),
				attribute.String("conditionID", task.ID.String()),
				attribute.String("error", err.Error()),
			),
		)
		s.log.WithError(err).WithFields(logrus.Fields{
			"assetID":           task.Asset.ID,
			"assetFacilityCode": task.Asset.Facility,
			"taskID":            task.ID.String(),
			"lastRev":           task.Revision,
			"key":               key,
		}).Warn("unable to write task status")

		return
	}

	s.log.WithFields(logrus.Fields{
		"assetID":           task.Asset.ID,
		"assetFacilityCode": task.Asset.Facility,
		"taskID":            task.ID.String(),
		"lastRev":           task.Revision,
		"currentRev":        rev,
		"key":               key,
	}).Trace("published task status")

	task.Revision = rev
}

func statusInfoJSON(s string) json.RawMessage {
	return []byte(fmt.Sprintf("{%q: %q}", "msg", s))
}
