package shared

import (
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/sourcegraph/sourcegraph/internal/env"
)

type SqliteConfig struct {
	SanityCheck       bool
	CacheDir          string
	CacheSizeMB       int
	NumCtagsProcesses int
	RequestBufferSize int
	ProcessingTimeout time.Duration
	Ctags             CtagsConfig
	RepositoryFetcher RepositoryFetcherConfig
}

func LoadSqliteConfig(baseConfig env.BaseConfig) SqliteConfig {
	return SqliteConfig{
		Ctags:             LoadCtagsConfig(baseConfig),
		SanityCheck:       baseConfig.GetBool("SANITY_CHECK", "false", "check that go-sqlite3 works then exit 0 if it's ok or 1 if not"),
		CacheDir:          baseConfig.Get("CACHE_DIR", "/tmp/symbols-cache", "directory in which to store cached symbols"),
		CacheSizeMB:       baseConfig.GetInt("SYMBOLS_CACHE_SIZE_MB", "100000", "maximum size of the disk cache (in megabytes)"),
		NumCtagsProcesses: baseConfig.GetInt("CTAGS_PROCESSES", strconv.Itoa(runtime.GOMAXPROCS(0)), "number of concurrent parser processes to run"),
		RequestBufferSize: baseConfig.GetInt("REQUEST_BUFFER_SIZE", "8192", "maximum size of buffered parser request channel"),
		ProcessingTimeout: baseConfig.GetInterval("PROCESSING_TIMEOUT", "2h", "maximum time to spend processing a repository"),
	}
}

type CtagsConfig struct {
	Command            string
	PatternLengthLimit int
	LogErrors          bool
	DebugLogs          bool
}

func LoadCtagsConfig(baseConfig env.BaseConfig) CtagsConfig {
	logCtagsErrorsDefault := "false"
	if os.Getenv("DEPLOY_TYPE") == "dev" {
		logCtagsErrorsDefault = "true"
	}

	return CtagsConfig{
		Command:            baseConfig.Get("CTAGS_COMMAND", "universal-ctags", "ctags command (should point to universal-ctags executable compiled with JSON and seccomp support)"),
		PatternLengthLimit: baseConfig.GetInt("CTAGS_PATTERN_LENGTH_LIMIT", "250", "the maximum length of the patterns output by ctags"),
		LogErrors:          baseConfig.GetBool("LOG_CTAGS_ERRORS", logCtagsErrorsDefault, "log ctags errors"),
		DebugLogs:          false,
	}
}

type RepositoryFetcherConfig struct {
	// The maximum sum of lengths of all paths in a single call to git archive. Without this limit, we
	// could hit the error "argument list too long" by exceeding the limit on the number of arguments to
	// a command enforced by the OS.
	//
	// Mac  : getconf ARG_MAX returns 1,048,576
	// Linux: getconf ARG_MAX returns 2,097,152
	//
	// We want to remain well under that limit, so defaulting to 100,000 seems safe (see the
	// MAX_TOTAL_PATHS_LENGTH environment variable below).
	MaxTotalPathsLength int
}

func LoadRepositoryFetcherConfig(baseConfig env.BaseConfig) RepositoryFetcherConfig {
	return RepositoryFetcherConfig{
		MaxTotalPathsLength: baseConfig.GetInt("MAX_TOTAL_PATHS_LENGTH", "100000", "maximum sum of lengths of all paths in a single call to git archive"),
	}
}
