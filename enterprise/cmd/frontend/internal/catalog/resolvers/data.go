package resolvers

import (
	"sort"

	"github.com/sourcegraph/sourcegraph/internal/database"
)

type usagePattern struct {
	query string
}

func newQueryUsagePattern(query string) usagePattern {
	return usagePattern{
		query: `repo:^github\.com/sourcegraph/sourcegraph$ ` + query,
	}
}

// TODO(sqs): dummy data
func dummyData(db database.DB) []*catalogComponentResolver {
	const (
		sourceRepo   = "github.com/sourcegraph/sourcegraph"
		sourceCommit = "2ada4911722e2c812cc4f1bbfb6d5d1756891392"
	)
	components := []*catalogComponentResolver{
		{
			kind:         "SERVICE",
			name:         "frontend",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"cmd/frontend", "enterprise/cmd/frontend"},
		},
		{
			kind:         "SERVICE",
			name:         "gitserver",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"cmd/gitserver"},
			usagePatterns: []usagePattern{
				newQueryUsagePattern(`lang:go \bgitserver\.Client\b patterntype:regexp`),
				newQueryUsagePattern(`lang:go \bgit\.[A-Z]\w+\(ctx, patterntype:regexp`),
			},
		},
		{
			kind:         "SERVICE",
			name:         "repo-updater",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"cmd/repo-updater", "enterprise/cmd/repo-updater"},
		},
		{
			kind:         "SERVICE",
			name:         "executor",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"enterprise/cmd/executor"},
		},
		{
			kind:         "SERVICE",
			name:         "precise-code-intel-worker",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"enterprise/cmd/precise-code-intel-worker"},
		},
		{
			kind:         "SERVICE",
			name:         "github-proxy",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"cmd/github-proxy"},
		},
		{
			kind:         "SERVICE",
			name:         "query-runner",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"cmd/query-runner"},
		},
		{
			kind:         "SERVICE",
			name:         "worker",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"cmd/worker", "enterprise/cmd/worker"},
		},
		{
			kind:         "SERVICE",
			name:         "server",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"cmd/server", "enterprise/cmd/server"},
		},
		{
			kind:         "SERVICE",
			name:         "symbols",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"cmd/symbols"},
		},
		{
			kind:         "SERVICE",
			name:         "sitemap",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"cmd/sitemap"},
		},
		{
			kind:         "TOOL",
			name:         "sg",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"dev/sg"},
			usagePatterns: []usagePattern{
				newQueryUsagePattern(`lang:markdown ` + "`" + `sg[` + "`" + `\s] patterntype:regexp`),
				newQueryUsagePattern(`lang:markdown (^|\s*\$ )sg\s patterntype:regexp`),
			},
		},
		{
			kind:         "LIBRARY",
			name:         "client-web",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"client/web"},
		},
		{
			kind:         "LIBRARY",
			name:         "client-browser",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"client/browser"},
		},
		{
			kind:         "LIBRARY",
			name:         "client-shared",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"client/shared"},
		},
		{
			kind:         "LIBRARY",
			name:         "wildcard",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"client/wildcard"},
			usagePatterns: []usagePattern{
				newQueryUsagePattern(`lang:typescript import @sourcegraph/wildcard patterntype:regexp`),
			},
		},
		{
			kind:         "LIBRARY",
			name:         "extension-api",
			sourceRepo:   sourceRepo,
			sourceCommit: sourceCommit,
			sourcePaths:  []string{"client/extension-api"},
			usagePatterns: []usagePattern{
				newQueryUsagePattern(`lang:typescript import from ['"]sourcegraph['"] patterntype:regexp`),
			},
		},
	}
	sort.Slice(components, func(i, j int) bool { return components[i].name < components[j].name })
	for _, c := range components {
		c.db = db
	}
	return components
}
