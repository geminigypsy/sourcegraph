package pings

import (
	"context"
	"encoding/json"
	"time"

	"github.com/inconshreveable/log15"

	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/database/dbutil"
	"github.com/sourcegraph/sourcegraph/internal/goroutine"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/sourcegraph/sourcegraph/internal/usagestats"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// NewInsightsPingEmitterJob will emit pings from Code Insights that involve enterprise features such as querying
// directly against the code insights database.
func NewInsightsPingEmitterJob(ctx context.Context, base dbutil.DB, insights dbutil.DB) goroutine.BackgroundRoutine {
	interval := time.Minute * 60
	e := InsightsPingEmitter{
		postgresDb: base,
		insightsDb: insights,
	}

	return goroutine.NewPeriodicGoroutine(ctx, interval,
		goroutine.NewHandlerWithErrorMessage("insights_pings_emitter", e.emit))
}

type InsightsPingEmitter struct {
	postgresDb dbutil.DB
	insightsDb dbutil.DB
}

func (e *InsightsPingEmitter) emit(ctx context.Context) error {
	log15.Info("Emitting Code Insights Pings")

	err := e.emitInsightTotalCounts(ctx)
	if err != nil {
		return errors.Wrap(err, "emitInsightTotalCounts")
	}
	err = e.emitIntervalCounts(ctx)
	if err != nil {
		return errors.Wrap(err, "emitIntervalCounts")
	}
	err = e.emitOrgVisibleInsightCounts(ctx)
	if err != nil {
		return errors.Wrap(err, "emitOrgVisibleInsightCounts")
	}
	return nil
}

func (e *InsightsPingEmitter) emitInsightTotalCounts(ctx context.Context) error {
	var counts types.InsightTotalCounts
	byViewType, err := e.GetTotalCountByViewType(ctx)
	if err != nil {
		return errors.Wrap(err, "GetTotalCountByViewType")
	}
	counts.ViewCounts = byViewType

	bySeriesType, err := e.GetTotalCountBySeriesType(ctx)
	if err != nil {
		return errors.Wrap(err, "GetTotalCountBySeriesType")
	}
	counts.SeriesCounts = bySeriesType

	byViewSeriesType, err := e.GetTotalCountByViewSeriesType(ctx)
	if err != nil {
		return errors.Wrap(err, "GetTotalCountByViewSeriesType")
	}
	counts.ViewSeriesCounts = byViewSeriesType

	marshal, err := json.Marshal(counts)
	if err != nil {
		return errors.Wrap(err, "Marshal")
	}

	err = e.SaveEvent(ctx, usagestats.InsightsTotalCountPingName, marshal)
	if err != nil {
		return errors.Wrap(err, "SaveEvent")
	}
	return nil
}

func (e *InsightsPingEmitter) emitIntervalCounts(ctx context.Context) error {
	counts, err := e.GetIntervalCounts(ctx)
	if err != nil {
		return errors.Wrap(err, "GetIntervalCounts")
	}

	marshal, err := json.Marshal(counts)
	if err != nil {
		return errors.Wrap(err, "Marshal")
	}

	err = e.SaveEvent(ctx, usagestats.InsightsIntervalCountsPingName, marshal)
	if err != nil {
		return errors.Wrap(err, "SaveEvent")
	}
	return nil
}

func (e *InsightsPingEmitter) emitOrgVisibleInsightCounts(ctx context.Context) error {
	counts, err := e.GetOrgVisibleInsightCounts(ctx)
	if err != nil {
		return errors.Wrap(err, "GetOrgVisibleInsightCounts")
	}

	marshal, err := json.Marshal(counts)
	if err != nil {
		return errors.Wrap(err, "Marshal")
	}

	err = e.SaveEvent(ctx, usagestats.InsightsOrgVisibleInsightsPingName, marshal)
	if err != nil {
		return errors.Wrap(err, "SaveEvent")
	}
	return nil
}

func (e *InsightsPingEmitter) SaveEvent(ctx context.Context, name string, argument json.RawMessage) error {
	store := database.EventLogs(e.postgresDb)

	err := store.Insert(ctx, &database.Event{
		Name:            name,
		UserID:          0,
		AnonymousUserID: "backend",
		Argument:        argument,
		Timestamp:       time.Now(),
		Source:          "BACKEND",
	})
	if err != nil {
		return err
	}
	return nil
}
