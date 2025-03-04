package graphqlbackend

import (
	"context"

	"github.com/google/zoekt"
	"github.com/graph-gophers/graphql-go"

	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/endpoint"
	"github.com/sourcegraph/sourcegraph/internal/search"
	"github.com/sourcegraph/sourcegraph/internal/search/run"
	"github.com/sourcegraph/sourcegraph/internal/trace"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/sourcegraph/schema"
)

type SearchArgs struct {
	Version     string
	PatternType *string
	Query       string

	// CodeMonitorID, if set, is the graphql-encoded ID of the code monitor
	// that is running the search. This will likely be removed in the future
	// once the worker can mutate and execute the search directly, but for now,
	// there are too many dependencies in frontend to do that. For anyone looking
	// to rip this out in the future, this should be possible once we can build
	// a static representation of our job tree independently of any resolvers.
	CodeMonitorID *graphql.ID

	// For tests
	Settings *schema.Settings
}

type SearchImplementer interface {
	Results(context.Context) (*SearchResultsResolver, error)
	//lint:ignore U1000 is used by graphql via reflection
	Stats(context.Context) (*searchResultsStats, error)

	Inputs() run.SearchInputs
}

// NewBatchSearchImplementer returns a SearchImplementer that provides search results and suggestions.
func NewBatchSearchImplementer(ctx context.Context, db database.DB, args *SearchArgs) (_ SearchImplementer, err error) {
	settings := args.Settings
	if settings == nil {
		var err error
		settings, err = DecodedViewerFinalSettings(ctx, db)
		if err != nil {
			return nil, err
		}
	}

	inputs, err := run.NewSearchInputs(
		ctx,
		db,
		args.Version,
		args.PatternType,
		args.Query,
		search.Batch,
		settings,
	)
	if err != nil {
		var queryErr *run.QueryError
		if errors.As(err, &queryErr) {
			return NewSearchAlertResolver(search.AlertForQuery(queryErr.Query, queryErr.Err)).wrapSearchImplementer(db), nil
		}
		return nil, err
	}

	return &searchResolver{
		db:           db,
		SearchInputs: inputs,
		zoekt:        search.Indexed(),
		searcherURLs: search.SearcherURLs(),
	}, nil
}

func (r *schemaResolver) Search(ctx context.Context, args *SearchArgs) (SearchImplementer, error) {
	return NewBatchSearchImplementer(ctx, r.db, args)
}

// searchResolver is a resolver for the GraphQL type `Search`
type searchResolver struct {
	SearchInputs *run.SearchInputs
	db           database.DB

	zoekt        zoekt.Streamer
	searcherURLs *endpoint.Map
}

func (r *searchResolver) Inputs() run.SearchInputs {
	return *r.SearchInputs
}

var MockDecodedViewerFinalSettings *schema.Settings

// DecodedViewerFinalSettings returns the final (merged) settings for the viewer
func DecodedViewerFinalSettings(ctx context.Context, db database.DB) (_ *schema.Settings, err error) {
	tr, ctx := trace.New(ctx, "decodedViewerFinalSettings", "")
	defer func() {
		tr.SetError(err)
		tr.Finish()
	}()
	if MockDecodedViewerFinalSettings != nil {
		return MockDecodedViewerFinalSettings, nil
	}

	cascade, err := (&schemaResolver{db: db}).ViewerSettings(ctx)
	if err != nil {
		return nil, err
	}

	return cascade.finalTyped(ctx)
}
