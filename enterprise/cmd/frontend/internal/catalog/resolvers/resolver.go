package resolvers

import (
	"context"
	"strings"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	gql "github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend/graphqlutil"
	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/database"
)

func NewRootResolver(db database.DB) gql.CatalogRootResolver {
	return &rootResolver{db: db}
}

type rootResolver struct {
	db database.DB
}

func (r *rootResolver) Catalog(context.Context) (gql.CatalogResolver, error) {
	return &catalogResolver{db: r.db}, nil
}

func (r *rootResolver) CatalogComponent(ctx context.Context, args *gql.CatalogComponentArgs) (gql.CatalogComponentResolver, error) {
	components := dummyData(r.db)
	for _, c := range components {
		if c.Name() == args.Name {
			return c, nil
		}
	}
	return nil, nil
}

func (r *rootResolver) NodeResolvers() map[string]gql.NodeByIDFunc {
	return map[string]gql.NodeByIDFunc{
		"CatalogComponent": func(ctx context.Context, id graphql.ID) (gql.Node, error) {
			components := dummyData(r.db)
			for _, c := range components {
				if c.ID() == id {
					return c, nil
				}
			}
			return nil, nil
		},
	}
}

type catalogResolver struct {
	db database.DB
}

func (r *catalogResolver) Components(ctx context.Context, args *gql.CatalogComponentsArgs) (gql.CatalogComponentConnectionResolver, error) {
	components := dummyData(r.db)

	var keep []gql.CatalogComponentResolver
	for _, c := range components {
		if args.Query == nil || strings.Contains(c.name, *args.Query) {
			keep = append(keep, c)
		}
	}

	return &catalogComponentConnectionResolver{
		components: keep,
	}, nil
}

type catalogComponentConnectionResolver struct {
	components []gql.CatalogComponentResolver
}

func (r *catalogComponentConnectionResolver) Nodes(ctx context.Context) ([]gql.CatalogComponentResolver, error) {
	return r.components, nil
}

func (r *catalogComponentConnectionResolver) TotalCount(ctx context.Context) (int32, error) {
	return int32(len(r.components)), nil
}

func (r *catalogComponentConnectionResolver) PageInfo(ctx context.Context) (*graphqlutil.PageInfo, error) {
	return graphqlutil.HasNextPage(false), nil // TODO(sqs)
}

type catalogComponentResolver struct {
	kind   gql.CatalogComponentKind
	name   string
	system *string

	sourceRepo, sourceCommit string
	sourcePaths              []string
	usagePatterns            []usagePattern

	db database.DB
}

func (r *catalogComponentResolver) ID() graphql.ID {
	return relay.MarshalID("CatalogComponent", r.name) // TODO(sqs)
}

func (r *catalogComponentResolver) Kind() gql.CatalogComponentKind {
	return r.kind
}

func (r *catalogComponentResolver) Name() string {
	return r.name
}

func (r *catalogComponentResolver) Owner(context.Context) (*gql.PersonResolver, error) {
	return nil, nil
}

func (r *catalogComponentResolver) System() *string {
	return r.system
}

func (r *catalogComponentResolver) Tags() []string {
	return []string{"my-tag1", "my-tag2"}
}

func (r *catalogComponentResolver) URL() string {
	return "/catalog/" + string(r.Name())
}

func (r *catalogComponentResolver) sourceRepoResolver(ctx context.Context) (*gql.RepositoryResolver, error) {
	// 🚨 SECURITY: database.Repos.Get uses the authzFilter under the hood and
	// filters out repositories that the user doesn't have access to.
	repo, err := r.db.Repos().GetByName(ctx, api.RepoName(r.sourceRepo))
	if err != nil {
		return nil, err
	}

	return gql.NewRepositoryResolver(r.db, repo), nil
}

func (r *catalogComponentResolver) SourceLocations(ctx context.Context) ([]*gql.GitTreeEntryResolver, error) {
	repoResolver, err := r.sourceRepoResolver(ctx)
	if err != nil {
		return nil, err
	}
	commitResolver := gql.NewGitCommitResolver(r.db, repoResolver, api.CommitID(r.sourceCommit), nil)
	var locs []*gql.GitTreeEntryResolver
	for _, sourcePath := range r.sourcePaths {

		locs = append(locs, gql.NewGitTreeEntryResolver(r.db, commitResolver, gql.CreateFileInfo(sourcePath, false)))
	}
	return locs, nil
}
