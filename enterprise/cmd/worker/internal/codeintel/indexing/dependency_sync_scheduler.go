package indexing

import (
	"context"
	"strconv"
	"time"

	"github.com/inconshreveable/log15"

	"github.com/sourcegraph/sourcegraph/enterprise/internal/codeintel/stores/dbstore"
	"github.com/sourcegraph/sourcegraph/internal/actor"
	dbstore2 "github.com/sourcegraph/sourcegraph/internal/codeintel/stores/dbstore"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/extsvc"
	"github.com/sourcegraph/sourcegraph/internal/observation"
	"github.com/sourcegraph/sourcegraph/internal/workerutil"
	"github.com/sourcegraph/sourcegraph/internal/workerutil/dbworker"
	dbworkerstore "github.com/sourcegraph/sourcegraph/internal/workerutil/dbworker/store"
	"github.com/sourcegraph/sourcegraph/lib/codeintel/precise"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

var schemeToExternalService = map[string]string{
	dbstore2.JVMPackagesScheme: extsvc.KindJVMPackages,
	dbstore2.NPMPackagesScheme: extsvc.KindNPMPackages,
}

// NewDependencySyncScheduler returns a new worker instance that processes
// records from lsif_dependency_syncing_jobs.
func NewDependencySyncScheduler(
	dbStore DBStore,
	workerStore dbworkerstore.Store,
	externalServiceStore ExternalServiceStore,
	metrics workerutil.WorkerMetrics,
) *workerutil.Worker {
	rootContext := actor.WithActor(context.Background(), &actor.Actor{Internal: true})

	handler := &dependencySyncSchedulerHandler{
		dbStore:     dbStore,
		workerStore: workerStore,
		extsvcStore: externalServiceStore,
	}

	return dbworker.NewWorker(rootContext, workerStore, handler, workerutil.WorkerOptions{
		Name:              "precise_code_intel_dependency_sync_scheduler_worker",
		NumHandlers:       1,
		Interval:          time.Second * 5,
		HeartbeatInterval: 1 * time.Second,
		Metrics:           metrics,
	})
}

type dependencySyncSchedulerHandler struct {
	dbStore     DBStore
	workerStore dbworkerstore.Store
	extsvcStore ExternalServiceStore
}

func (h *dependencySyncSchedulerHandler) Handle(ctx context.Context, record workerutil.Record) error {
	if !autoIndexingEnabled() {
		return nil
	}

	job := record.(dbstore.DependencySyncingJob)

	scanner, err := h.dbStore.ReferencesForUpload(ctx, job.UploadID)
	if err != nil {
		return errors.Wrap(err, "dbstore.ReferencesForUpload")
	}
	defer func() {
		if closeErr := scanner.Close(); closeErr != nil {
			err = errors.Append(err, errors.Wrap(closeErr, "dbstore.ReferencesForUpload.Close"))
		}
	}()

	var (
		kinds                      = map[string]struct{}{}
		oldDependencyReposInserted int
		newDependencyReposInserted int
		errs                       []error
	)

	for {
		packageReference, exists, err := scanner.Next()
		if err != nil {
			return errors.Wrap(err, "dbstore.ReferencesForUpload.Next")
		}
		if !exists {
			break
		}

		pkg := precise.Package{
			Scheme:  packageReference.Package.Scheme,
			Name:    packageReference.Package.Name,
			Version: packageReference.Package.Version,
		}

		extsvcKind, ok := schemeToExternalService[packageReference.Scheme]
		// add entry for empty string/kind here so dependencies such as lsif-go ones still get
		// an associated dependency indexing job
		kinds[extsvcKind] = struct{}{}
		if !ok {
			continue
		}

		new, err := h.insertDependencyRepo(ctx, pkg)
		if err != nil {
			errs = append(errs, err)
		} else if new {
			newDependencyReposInserted++
		} else {
			oldDependencyReposInserted++
		}
	}

	var nextSync time.Time
	// If len == 0, it will return all external services, which we definitely don't want.
	if len(kindsToArray(kinds)) > 0 {
		nextSync = time.Now()
		externalServices, err := h.extsvcStore.List(ctx, database.ExternalServicesListOptions{
			Kinds: kindsToArray(kinds),
		})
		if err != nil {
			if len(errs) == 0 {
				return errors.Wrap(err, "dbstore.List")
			} else {
				return errors.Append(err, errs...)
			}
		}

		log15.Info("syncing external services",
			"upload", job.UploadID, "numExtSvc", len(externalServices), "job", job.ID, "schemaKinds", kinds,
			"newRepos", newDependencyReposInserted, "existingInserts", oldDependencyReposInserted)

		for _, externalService := range externalServices {
			externalService.NextSyncAt = nextSync
			err := h.extsvcStore.Upsert(ctx, externalService)
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "extsvcStore.Upsert: error setting next_sync_at for external service %d - %s", externalService.ID, externalService.DisplayName))
			}
		}
	} else {
		log15.Info("no package schema kinds to sync external services for", "upload", job.UploadID, "job", job.ID)
	}

	shouldIndex, err := h.shouldIndexDependencies(ctx, h.dbStore, job.UploadID)
	if err != nil {
		return err
	}

	if shouldIndex {
		// If we saw a kind that's not in schemeToExternalService, then kinds contains an empty string key
		for kind := range kinds {
			if _, err := h.dbStore.InsertDependencyIndexingJob(ctx, job.UploadID, kind, nextSync); err != nil {
				errs = append(errs, errors.Wrap(err, "dbstore.InsertDependencyIndexingJob"))
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}

	if len(errs) == 1 {
		return errs[0]
	}

	return errors.Append(nil, errs...)
}

func (h *dependencySyncSchedulerHandler) insertDependencyRepo(ctx context.Context, pkg precise.Package) (new bool, err error) {
	ctx, endObservation := dependencyReposOps.InsertCloneableDependencyRepo.With(ctx, &err, observation.Args{
		MetricLabelValues: []string{pkg.Scheme},
	})
	defer func() {
		endObservation(1, observation.Args{MetricLabelValues: []string{strconv.FormatBool(new)}})
	}()

	new, err = h.dbStore.InsertCloneableDependencyRepo(ctx, pkg)
	if err != nil {
		return new, errors.Wrap(err, "dbstore.InsertCloneableDependencyRepos")
	}
	return new, nil
}

// shouldIndexDependencies returns true if the given upload should undergo dependency
// indexing. Currently, we're only enabling dependency indexing for a repositories that
// were indexed via lsif-go, lsif-java and lsif-tsc.
func (h *dependencySyncSchedulerHandler) shouldIndexDependencies(ctx context.Context, store DBStore, uploadID int) (bool, error) {
	upload, _, err := store.GetUploadByID(ctx, uploadID)
	if err != nil {
		return false, errors.Wrap(err, "dbstore.GetUploadByID")
	}

	return upload.Indexer == "lsif-go" || upload.Indexer == "lsif-java" || upload.Indexer == "lsif-tsc", nil
}

func kindsToArray(k map[string]struct{}) (s []string) {
	for kind := range k {
		if kind != "" {
			s = append(s, kind)
		}
	}
	return
}
