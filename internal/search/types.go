package search

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/zoekt"
	zoektquery "github.com/google/zoekt/query"
	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/endpoint"
	"github.com/sourcegraph/sourcegraph/internal/search/filter"
	"github.com/sourcegraph/sourcegraph/internal/search/query"
	"github.com/sourcegraph/sourcegraph/internal/search/result"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/sourcegraph/sourcegraph/schema"
)

type SymbolsParameters struct {
	// Repo is the name of the repository to search in.
	Repo api.RepoName `json:"repo"`

	// CommitID is the commit to search in.
	CommitID api.CommitID `json:"commitID"`

	// Query is the search query.
	Query string

	// IsRegExp if true will treat the Pattern as a regular expression.
	IsRegExp bool

	// IsCaseSensitive if false will ignore the case of query and file pattern
	// when finding matches.
	IsCaseSensitive bool

	// IncludePatterns is a list of regexes that symbol's file paths
	// need to match to get included in the result
	//
	// The patterns are ANDed together; a file's path must match all patterns
	// for it to be kept. That is also why it is a list (unlike the singular
	// ExcludePattern); it is not possible in general to construct a single
	// glob or Go regexp that represents multiple such patterns ANDed together.
	IncludePatterns []string

	// ExcludePattern is an optional regex that symbol's file paths
	// need to match to get included in the result
	ExcludePattern string

	// First indicates that only the first n symbols should be returned.
	First int
}

// GlobalSearchMode designates code paths which optimize performance for global
// searches, i.e., literal or regexp, indexed searches without repo: filter.
type GlobalSearchMode int

const (
	DefaultMode GlobalSearchMode = iota

	// ZoektGlobalSearch designates a performance optimised code path for indexed
	// searches. For a global search we don't need to resolve repos before searching
	// shards on Zoekt, instead we can resolve repos and call Zoekt concurrently.
	//
	// Note: Even for a global search we have to resolve repos to filter search results
	// returned by Zoekt.
	ZoektGlobalSearch

	// SearcherOnly designated a code path on which we skip indexed search, even if
	// the user specified index:yes. SearcherOnly is used in conjunction with
	// ZoektGlobalSearch and designates the non-indexed part of the performance
	// optimised code path.
	SearcherOnly

	// SkipUnindexed disables content, path, and symbol search. Used:
	// (1) in conjunction with ZoektGlobalSearch on Sourcegraph.com.
	// (2) when a query does not specify any patterns, include patterns, or exclude pattern.
	SkipUnindexed
)

var globalSearchModeStrings = map[GlobalSearchMode]string{
	ZoektGlobalSearch: "ZoektGlobalSearch",
	SearcherOnly:      "SearcherOnly",
	SkipUnindexed:     "SkipUnindexed",
}

func (m GlobalSearchMode) String() string {
	if s, ok := globalSearchModeStrings[m]; ok {
		return s
	}
	return "None"
}

type IndexedRequestType string

const (
	TextRequest   IndexedRequestType = "text"
	SymbolRequest IndexedRequestType = "symbol"
)

// ZoektParameters contains all the inputs to run a Zoekt indexed search.
type ZoektParameters struct {
	Query          zoektquery.Q
	Typ            IndexedRequestType
	FileMatchLimit int32
	Select         filter.SelectPath

	Zoekt zoekt.Streamer
}

// SearcherParameters the inputs for a search fulfilled by the Searcher service
// (cmd/searcher). Searcher fulfills (1) unindexed literal and regexp searches
// and (2) structural search requests.
type SearcherParameters struct {
	SearcherURLs *endpoint.Map
	PatternInfo  *TextPatternInfo

	// UseFullDeadline indicates that the search should try do as much work as
	// it can within context.Deadline. If false the search should try and be
	// as fast as possible, even if a "slow" deadline is set.
	//
	// For example searcher will wait to full its archive cache for a
	// repository if this field is true. Another example is we set this field
	// to true if the user requests a specific timeout or maximum result size.
	UseFullDeadline bool
}

// TextParameters are the parameters passed to a search backend. It contains the Pattern
// to search for, as well as the hydrated list of repository revisions to
// search. It defines behavior for text search on repository names, file names, and file content.
type TextParameters struct {
	PatternInfo *TextPatternInfo
	RepoOptions RepoOptions
	Features    Features
	ResultTypes result.Types
	Timeout     time.Duration

	Repos []*RepositoryRevisions

	// perf: For global queries, we only resolve private repos.
	UserPrivateRepos []types.MinimalRepo
	Mode             GlobalSearchMode

	// Query is the parsed query from the user. You should be using Pattern
	// instead, but Query is useful for checking extra fields that are set and
	// ignored by Pattern, such as index:no
	Query query.Q

	// UseFullDeadline indicates that the search should try do as much work as
	// it can within context.Deadline. If false the search should try and be
	// as fast as possible, even if a "slow" deadline is set.
	//
	// For example searcher will wait to full its archive cache for a
	// repository if this field is true. Another example is we set this field
	// to true if the user requests a specific timeout or maximum result size.
	UseFullDeadline bool

	Zoekt        zoekt.Streamer
	SearcherURLs *endpoint.Map
}

// TextPatternInfo is the struct used by vscode pass on search queries. Keep it in
// sync with pkg/searcher/protocol.PatternInfo.
type TextPatternInfo struct {
	Pattern         string
	IsNegated       bool
	IsRegExp        bool
	IsStructuralPat bool
	CombyRule       string
	IsWordMatch     bool
	IsCaseSensitive bool
	FileMatchLimit  int32
	Index           query.YesNoOnly
	Select          filter.SelectPath

	// We do not support IsMultiline
	// IsMultiline     bool
	IncludePatterns []string
	ExcludePattern  string

	FilePatternsReposMustInclude []string
	FilePatternsReposMustExclude []string

	PathPatternsAreCaseSensitive bool

	PatternMatchesContent bool
	PatternMatchesPath    bool

	Languages []string
}

func (p *TextPatternInfo) String() string {
	args := []string{fmt.Sprintf("%q", p.Pattern)}
	if p.IsRegExp {
		args = append(args, "re")
	}
	if p.IsStructuralPat {
		if p.CombyRule != "" {
			args = append(args, fmt.Sprintf("comby:%s", p.CombyRule))
		} else {
			args = append(args, "comby")
		}
	}
	if p.IsWordMatch {
		args = append(args, "word")
	}
	if p.IsCaseSensitive {
		args = append(args, "case")
	}
	if !p.PatternMatchesContent {
		args = append(args, "nocontent")
	}
	if !p.PatternMatchesPath {
		args = append(args, "nopath")
	}
	if p.FileMatchLimit > 0 {
		args = append(args, fmt.Sprintf("filematchlimit:%d", p.FileMatchLimit))
	}
	for _, lang := range p.Languages {
		args = append(args, fmt.Sprintf("lang:%s", lang))
	}

	for _, inc := range p.FilePatternsReposMustInclude {
		args = append(args, fmt.Sprintf("repositoryPathPattern:%s", inc))
	}
	for _, dec := range p.FilePatternsReposMustExclude {
		args = append(args, fmt.Sprintf("-repositoryPathPattern:%s", dec))
	}

	path := "f"
	if p.PathPatternsAreCaseSensitive {
		path = "F"
	}
	if p.ExcludePattern != "" {
		args = append(args, fmt.Sprintf("-%s:%q", path, p.ExcludePattern))
	}
	for _, inc := range p.IncludePatterns {
		args = append(args, fmt.Sprintf("%s:%q", path, inc))
	}

	return fmt.Sprintf("TextPatternInfo{%s}", strings.Join(args, ","))
}

// Features describe feature flags for a request. This is state that differs
// across users and time. It is created based on user feature flags and
// configuration.
//
// The Feature struct should be initialized once per search request early on.
//
// The default value for a Feature should be the go zero value, such that
// creating an empty Feature struct represents the usual search
// experience. This is to avoid needing to update a large number of tests when
// a new feature flag is introduced, and instead changes are localized to this
// struct and read sites of a flag.
type Features struct {
	// ContentBasedLangFilters when true will use the language detected from
	// the content of the file, rather than just file name patterns. This is
	// currently just supported by Zoekt.
	ContentBasedLangFilters bool
}

type RepoOptions struct {
	RepoFilters              []string
	MinusRepoFilters         []string
	Dependencies             []string
	CaseSensitiveRepoFilters bool
	SearchContextSpec        string
	UserSettings             *schema.Settings
	NoForks                  bool
	OnlyForks                bool
	NoArchived               bool
	OnlyArchived             bool
	CommitAfter              string
	Visibility               query.RepoVisibility
	Limit                    int
	Cursors                  []*types.Cursor
	Query                    query.Q
}

func (op *RepoOptions) String() string {
	var b strings.Builder
	if len(op.RepoFilters) == 0 {
		b.WriteString("r=[]")
	}
	for i, r := range op.RepoFilters {
		if i != 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.Quote(r))
	}

	if len(op.MinusRepoFilters) > 0 {
		_, _ = fmt.Fprintf(&b, " -r=%v", op.MinusRepoFilters)
	}
	if op.CommitAfter != "" {
		_, _ = fmt.Fprintf(&b, " CommitAfter=%q", op.CommitAfter)
	}

	if op.CaseSensitiveRepoFilters {
		b.WriteString(" CaseSensitiveRepoFilters")
	}

	if op.NoForks {
		b.WriteString(" NoForks")
	}
	if op.OnlyForks {
		b.WriteString(" OnlyForks")
	}
	if op.NoArchived {
		b.WriteString(" NoArchived")
	}
	if op.OnlyArchived {
		b.WriteString(" OnlyArchived")
	}
	if op.Visibility != query.Any {
		b.WriteString(" Visibility" + string(op.Visibility))
	}

	return b.String()
}
