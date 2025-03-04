package database

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/keegancsmith/sqlf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/envvar"
	"github.com/sourcegraph/sourcegraph/internal/actor"
	"github.com/sourcegraph/sourcegraph/internal/conf"
	"github.com/sourcegraph/sourcegraph/internal/database/dbtest"
	"github.com/sourcegraph/sourcegraph/internal/encryption"
	et "github.com/sourcegraph/sourcegraph/internal/encryption/testing"
	"github.com/sourcegraph/sourcegraph/internal/errcode"
	"github.com/sourcegraph/sourcegraph/internal/extsvc"
	"github.com/sourcegraph/sourcegraph/internal/timeutil"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/sourcegraph/sourcegraph/internal/types/typestest"
	"github.com/sourcegraph/sourcegraph/schema"
)

func TestExternalServicesListOptions_sqlConditions(t *testing.T) {
	tests := []struct {
		name                 string
		noNamespace          bool
		excludeNamespaceUser bool
		namespaceUserID      int32
		namespaceOrgID       int32
		kinds                []string
		afterID              int64
		wantQuery            string
		onlyCloudDefault     bool
		noCachedWebhooks     bool
		wantArgs             []interface{}
	}{
		{
			name:      "no condition",
			wantQuery: "deleted_at IS NULL",
		},
		{
			name:      "only one kind: GitHub",
			kinds:     []string{extsvc.KindGitHub},
			wantQuery: "deleted_at IS NULL AND kind IN ($1)",
			wantArgs:  []interface{}{extsvc.KindGitHub},
		},
		{
			name:      "two kinds: GitHub and GitLab",
			kinds:     []string{extsvc.KindGitHub, extsvc.KindGitLab},
			wantQuery: "deleted_at IS NULL AND kind IN ($1 , $2)",
			wantArgs:  []interface{}{extsvc.KindGitHub, extsvc.KindGitLab},
		},
		{
			name:            "has namespace user ID",
			namespaceUserID: 1,
			wantQuery:       "deleted_at IS NULL AND namespace_user_id = $1",
			wantArgs:        []interface{}{int32(1)},
		},
		{
			name:           "has namespace org ID",
			namespaceOrgID: 1,
			wantQuery:      "deleted_at IS NULL AND namespace_org_id = $1",
			wantArgs:       []interface{}{int32(1)},
		},
		{
			name:            "want no namespace",
			noNamespace:     true,
			namespaceUserID: 1,
			namespaceOrgID:  42,
			wantQuery:       "deleted_at IS NULL AND namespace_user_id IS NULL AND namespace_org_id IS NULL",
		},
		{
			name:                 "want exclude namespace user",
			excludeNamespaceUser: true,
			wantQuery:            "deleted_at IS NULL AND namespace_user_id IS NULL",
		},
		{
			name:      "has after ID",
			afterID:   10,
			wantQuery: "deleted_at IS NULL AND id < $1",
			wantArgs:  []interface{}{int64(10)},
		},
		{
			name:             "has OnlyCloudDefault",
			onlyCloudDefault: true,
			wantQuery:        "deleted_at IS NULL AND cloud_default = true",
		},
		{
			name:             "has noCachedWebhooks",
			noCachedWebhooks: true,
			wantQuery:        "deleted_at IS NULL AND has_webhooks IS NULL",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := ExternalServicesListOptions{
				NoNamespace:          test.noNamespace,
				ExcludeNamespaceUser: test.excludeNamespaceUser,
				NamespaceUserID:      test.namespaceUserID,
				NamespaceOrgID:       test.namespaceOrgID,
				Kinds:                test.kinds,
				AfterID:              test.afterID,
				OnlyCloudDefault:     test.onlyCloudDefault,
				NoCachedWebhooks:     test.noCachedWebhooks,
			}
			q := sqlf.Join(opts.sqlConditions(), "AND")
			if diff := cmp.Diff(test.wantQuery, q.Query(sqlf.PostgresBindVar)); diff != "" {
				t.Fatalf("query mismatch (-want +got):\n%s", diff)
			} else if diff = cmp.Diff(test.wantArgs, q.Args()); diff != "" {
				t.Fatalf("args mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExternalServicesStore_ValidateConfig(t *testing.T) {
	// Can't currently run in parallel because of global mocks
	db := dbtest.NewDB(t)

	tests := []struct {
		name            string
		kind            string
		config          string
		namespaceUserID int32
		namespaceOrgID  int32
		setup           func(t *testing.T)
		wantErr         string
	}{
		{
			name:    "0 errors - GitHub.com",
			kind:    extsvc.KindGitHub,
			config:  `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
			wantErr: "<nil>",
		},
		{
			name:    "0 errors - GitLab.com",
			kind:    extsvc.KindGitLab,
			config:  `{"url": "https://github.com", "projectQuery": ["none"], "token": "abc"}`,
			wantErr: "<nil>",
		},
		{
			name:    "0 errors - Bitbucket.org",
			kind:    extsvc.KindBitbucketCloud,
			config:  `{"url": "https://bitbucket.org", "username": "ceo", "appPassword": "abc"}`,
			wantErr: "<nil>",
		},
		{
			name:    "1 error",
			kind:    extsvc.KindGitHub,
			config:  `{"repositoryQuery": ["none"], "token": "fake"}`,
			wantErr: "url is required",
		},
		{
			name:    "2 errors",
			kind:    extsvc.KindGitHub,
			config:  `{"url": "https://github.com", "repositoryQuery": ["none"], "token": ""}`,
			wantErr: "2 errors occurred:\n\t* token: String length must be greater than or equal to 1\n\t* at least one of token or githubAppInstallationID must be set",
		},
		{
			name:   "no conflicting rate limit",
			kind:   extsvc.KindGitHub,
			config: `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc", "rateLimit": {"enabled": true, "requestsPerHour": 5000}}`,
			setup: func(t *testing.T) {
				t.Cleanup(func() {
					Mocks.ExternalServices.List = nil
				})
				Mocks.ExternalServices.List = func(opt ExternalServicesListOptions) ([]*types.ExternalService, error) {
					return nil, nil
				}
			},
			wantErr: "<nil>",
		},
		{
			name:   "conflicting rate limit",
			kind:   extsvc.KindGitHub,
			config: `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc", "rateLimit": {"enabled": true, "requestsPerHour": 5000}}`,
			setup: func(t *testing.T) {
				t.Cleanup(func() {
					Mocks.ExternalServices.List = nil
				})
				Mocks.ExternalServices.List = func(opt ExternalServicesListOptions) ([]*types.ExternalService, error) {
					return []*types.ExternalService{
						{
							ID:          1,
							Kind:        extsvc.KindGitHub,
							DisplayName: "GITHUB 1",
							Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc", "rateLimit": {"enabled": true, "requestsPerHour": 5000}}`,
						},
					}, nil
				}
			},
			wantErr: "existing external service, \"GITHUB 1\", already has a rate limit set",
		},
		{
			name:            "prevent code hosts that are not allowed",
			kind:            extsvc.KindGitHub,
			config:          `{"url": "https://github.example.com", "repositoryQuery": ["none"], "token": "abc"}`,
			namespaceUserID: 1,
			wantErr:         `external service only allowed for https://github.com/ and https://gitlab.com/`,
		},
		{
			name:           "prevent code hosts that are not allowed for organizations",
			kind:           extsvc.KindGitHub,
			config:         `{"url": "https://github.example.com", "repositoryQuery": ["none"], "token": "abc"}`,
			namespaceOrgID: 1,
			wantErr:        `external service only allowed for https://github.com/ and https://gitlab.com/`,
		},
		{
			name:            "gjson handles comments",
			kind:            extsvc.KindGitHub,
			config:          `{"url": "https://github.com", "token": "abc", "repositoryQuery": ["affiliated"]} // comment`,
			namespaceUserID: 1,
			wantErr:         "<nil>",
		},
		{
			name:            "prevent disallowed repositoryPathPattern field",
			kind:            extsvc.KindGitHub,
			config:          `{"url": "https://github.com", "repositoryPathPattern": "github/{nameWithOwner}"}`,
			namespaceUserID: 1,
			wantErr:         `field "repositoryPathPattern" is not allowed in a user-added external service`,
		},
		{
			name:            "prevent disallowed nameTransformations field",
			kind:            extsvc.KindGitHub,
			config:          `{"url": "https://github.com", "nameTransformations": [{"regex": "\\.d/","replacement": "/"},{"regex": "-git$","replacement": ""}]}`,
			namespaceUserID: 1,
			wantErr:         `field "nameTransformations" is not allowed in a user-added external service`,
		},
		{
			name:            "prevent disallowed rateLimit field",
			kind:            extsvc.KindGitHub,
			config:          `{"url": "https://github.com", "rateLimit": {}}`,
			namespaceUserID: 1,
			wantErr:         `field "rateLimit" is not allowed in a user-added external service`,
		},
		{
			name:            "duplicate kinds not allowed for user owned services",
			kind:            extsvc.KindGitHub,
			config:          `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
			namespaceUserID: 1,
			setup: func(t *testing.T) {
				t.Cleanup(func() {
					Mocks.ExternalServices.List = nil
				})
				Mocks.ExternalServices.List = func(opt ExternalServicesListOptions) ([]*types.ExternalService, error) {
					return []*types.ExternalService{
						{
							ID:          1,
							Kind:        extsvc.KindGitHub,
							DisplayName: "GITHUB 1",
							Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
						},
					}, nil
				}
			},
			wantErr: `existing external service, "GITHUB 1", of same kind already added`,
		},
		{
			name:           "duplicate kinds not allowed for org owned services",
			kind:           extsvc.KindGitHub,
			config:         `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
			namespaceOrgID: 1,
			setup: func(t *testing.T) {
				t.Cleanup(func() {
					Mocks.ExternalServices.List = nil
				})
				Mocks.ExternalServices.List = func(opt ExternalServicesListOptions) ([]*types.ExternalService, error) {
					return []*types.ExternalService{
						{
							ID:          1,
							Kind:        extsvc.KindGitHub,
							DisplayName: "GITHUB 1",
							Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
						},
					}, nil
				}
			},
			wantErr: `existing external service, "GITHUB 1", of same kind already added`,
		},
		{
			name:    "1 errors - GitHub.com",
			kind:    extsvc.KindGitHub,
			config:  `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "` + types.RedactedSecret + `"}`,
			wantErr: "unable to write external service config as it contains redacted fields, this is likely a bug rather than a problem with your config",
		},
		{
			name:    "1 errors - GitLab.com",
			kind:    extsvc.KindGitLab,
			config:  `{"url": "https://github.com", "projectQuery": ["none"], "token": "` + types.RedactedSecret + `"}`,
			wantErr: "unable to write external service config as it contains redacted fields, this is likely a bug rather than a problem with your config",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			_, err := ExternalServices(db).ValidateConfig(context.Background(), ValidateExternalServiceConfigOptions{
				Kind:            test.kind,
				Config:          test.config,
				NamespaceUserID: test.namespaceUserID,
				NamespaceOrgID:  test.namespaceOrgID,
			})
			gotErr := fmt.Sprintf("%v", err)
			if gotErr != test.wantErr {
				t.Errorf("error: want %q but got %q", test.wantErr, gotErr)
			}
		})
	}
}

func TestExternalServicesStore_Create(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	db := dbtest.NewDB(t)
	ctx := context.Background()

	envvar.MockSourcegraphDotComMode(true)
	defer envvar.MockSourcegraphDotComMode(false)

	user, err := Users(db).Create(ctx,
		NewUser{
			Email:           "alice@example.com",
			Username:        "alice",
			Password:        "password",
			EmailIsVerified: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	displayName := "Acme org"
	org, err := Orgs(db).Create(ctx, "acme", &displayName)
	if err != nil {
		t.Fatal(err)
	}

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}

	tests := []struct {
		name             string
		externalService  *types.ExternalService
		wantUnrestricted bool
		wantHasWebhooks  bool
	}{
		{
			name: "with webhooks",
			externalService: &types.ExternalService{
				Kind:            extsvc.KindGitHub,
				DisplayName:     "GITHUB #1",
				Config:          `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc", "webhooks": [{"org": "org", "secret": "secret"}]}`,
				NamespaceUserID: user.ID,
			},
			wantUnrestricted: false,
			wantHasWebhooks:  true,
		},
		{
			name: "without authorization",
			externalService: &types.ExternalService{
				Kind:            extsvc.KindGitHub,
				DisplayName:     "GITHUB #1",
				Config:          `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
				NamespaceUserID: user.ID,
			},
			wantUnrestricted: false,
			wantHasWebhooks:  false,
		},
		{
			name: "with authorization",
			externalService: &types.ExternalService{
				Kind:            extsvc.KindGitHub,
				DisplayName:     "GITHUB #2",
				Config:          `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc", "authorization": {}}`,
				NamespaceUserID: user.ID,
			},
			wantUnrestricted: false,
			wantHasWebhooks:  false,
		},
		{
			name: "with authorization in comments",
			externalService: &types.ExternalService{
				Kind:        extsvc.KindGitHub,
				DisplayName: "GITHUB #3",
				Config: `
{
	"url": "https://github.com",
	"repositoryQuery": ["none"],
	"token": "abc",
	// "authorization": {}
}`,
				NamespaceUserID: user.ID,
			},
			wantUnrestricted: false,
		},

		{
			name: "Cloud: auto-add authorization to code host connections for GitHub",
			externalService: &types.ExternalService{
				Kind:            extsvc.KindGitHub,
				DisplayName:     "GITHUB #4",
				Config:          `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
				NamespaceUserID: user.ID,
			},
			wantUnrestricted: false,
			wantHasWebhooks:  false,
		},
		{
			name: "Cloud: auto-add authorization to code host connections for GitLab",
			externalService: &types.ExternalService{
				Kind:            extsvc.KindGitLab,
				DisplayName:     "GITLAB #1",
				Config:          `{"url": "https://gitlab.com", "projectQuery": ["none"], "token": "abc"}`,
				NamespaceUserID: user.ID,
			},
			wantUnrestricted: false,
			wantHasWebhooks:  false,
		},
		{
			name: "Cloud: support org namespace on code host connections for GitHub",
			externalService: &types.ExternalService{
				Kind:           extsvc.KindGitHub,
				DisplayName:    "GITHUB #4",
				Config:         `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
				NamespaceOrgID: org.ID,
			},
			wantUnrestricted: false,
			wantHasWebhooks:  false,
		},
		{
			name: "Cloud: support org namespace on code host connections for GitLab",
			externalService: &types.ExternalService{
				Kind:           extsvc.KindGitLab,
				DisplayName:    "GITLAB #1",
				Config:         `{"url": "https://gitlab.com", "projectQuery": ["none"], "token": "abc"}`,
				NamespaceOrgID: org.ID,
			},
			wantUnrestricted: false,
			wantHasWebhooks:  false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ExternalServices(db).Create(ctx, confGet, test.externalService)
			if err != nil {
				t.Fatal(err)
			}

			// Should get back the same one
			got, err := ExternalServices(db).GetByID(ctx, test.externalService.ID)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.externalService, got); diff != "" {
				t.Fatalf("Mismatch (-want +got):\n%s", diff)
			}

			if test.wantUnrestricted != got.Unrestricted {
				t.Fatalf("Want unrestricted = %v, but got %v", test.wantUnrestricted, got.Unrestricted)
			}

			if got.HasWebhooks == nil {
				t.Fatal("has_webhooks must not be null")
			} else if *got.HasWebhooks != test.wantHasWebhooks {
				t.Fatalf("Wanted has_webhooks = %v, but got %v", test.wantHasWebhooks, *got.HasWebhooks)
			}

			err = ExternalServices(db).Delete(ctx, test.externalService.ID)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExternalServicesStore_CreateWithTierEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	db := dbtest.NewDB(t)

	ctx := context.Background()
	confGet := func() *conf.Unified { return &conf.Unified{} }
	es := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
	}
	store := ExternalServices(db)
	BeforeCreateExternalService = func(ctx context.Context, _ ExternalServiceStore) error {
		return errcode.NewPresentationError("test plan limit exceeded")
	}
	t.Cleanup(func() { BeforeCreateExternalService = nil })
	if err := store.Create(ctx, confGet, es); err == nil {
		t.Fatal("expected an error, got none")
	}
}

func TestExternalServicesStore_Update(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	db := dbtest.NewDB(t)
	ctx := context.Background()

	envvar.MockSourcegraphDotComMode(true)
	defer envvar.MockSourcegraphDotComMode(false)

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	es := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc", "authorization": {}}`,
	}
	err := ExternalServices(db).Create(ctx, confGet, es)
	if err != nil {
		t.Fatal(err)
	}

	// NOTE: The order of tests matters
	tests := []struct {
		name             string
		update           *ExternalServiceUpdate
		wantUnrestricted bool
		wantCloudDefault bool
		wantHasWebhooks  bool
	}{
		{
			name: "update with authorization",
			update: &ExternalServiceUpdate{
				DisplayName: strptr("GITHUB (updated) #1"),
				Config:      strptr(`{"url": "https://github.com", "repositoryQuery": ["none"], "token": "def", "authorization": {}, "webhooks": [{"org": "org", "secret": "secret"}]}`),
			},
			wantUnrestricted: false,
			wantCloudDefault: false,
			wantHasWebhooks:  true,
		},
		{
			name: "update without authorization",
			update: &ExternalServiceUpdate{
				DisplayName: strptr("GITHUB (updated) #2"),
				Config:      strptr(`{"url": "https://github.com", "repositoryQuery": ["none"], "token": "def"}`),
			},
			wantUnrestricted: false,
			wantCloudDefault: false,
			wantHasWebhooks:  false,
		},
		{
			name: "update with authorization in comments",
			update: &ExternalServiceUpdate{
				DisplayName: strptr("GITHUB (updated) #3"),
				Config: strptr(`
{
	"url": "https://github.com",
	"repositoryQuery": ["none"],
	"token": "def",
	// "authorization": {}
}`),
			},
			wantUnrestricted: false,
			wantCloudDefault: false,
			wantHasWebhooks:  false,
		},
		{
			name: "set cloud_default true",
			update: &ExternalServiceUpdate{
				DisplayName:  strptr("GITHUB (updated) #3"),
				CloudDefault: boolptr(true),
				Config: strptr(`
{
	"url": "https://github.com",
	"repositoryQuery": ["none"],
	"token": "def",
	"authorization": {},
	"webhooks": [{"org": "org", "secret": "secret"}]
}`),
			},
			wantUnrestricted: false,
			wantCloudDefault: true,
			wantHasWebhooks:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err = ExternalServices(db).Update(ctx, nil, es.ID, test.update)
			if err != nil {
				t.Fatal(err)
			}

			// Get and verify update
			got, err := ExternalServices(db).GetByID(ctx, es.ID)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(*test.update.DisplayName, got.DisplayName); diff != "" {
				t.Fatalf("DisplayName mismatch (-want +got):\n%s", diff)
			} else if diff = cmp.Diff(*test.update.Config, got.Config); diff != "" {
				t.Fatalf("Config mismatch (-want +got):\n%s", diff)
			} else if got.UpdatedAt.Equal(es.UpdatedAt) {
				t.Fatalf("UpdateAt: want to be updated but not")
			}

			if test.wantUnrestricted != got.Unrestricted {
				t.Fatalf("Want unrestricted = %v, but got %v", test.wantUnrestricted, got.Unrestricted)
			}

			if test.wantCloudDefault != got.CloudDefault {
				t.Fatalf("Want cloud_default = %v, but got %v", test.wantCloudDefault, got.CloudDefault)
			}

			if got.HasWebhooks == nil {
				t.Fatal("has_webhooks is unexpectedly null")
			} else if test.wantHasWebhooks != *got.HasWebhooks {
				t.Fatalf("Want has_webhooks = %v, but got %v", test.wantHasWebhooks, *got.HasWebhooks)
			}
		})
	}
}

func TestUpsertAuthorizationToExternalService(t *testing.T) {
	tests := []struct {
		name   string
		kind   string
		config string
		want   string
	}{
		{
			name: "github with authorization",
			kind: extsvc.KindGitHub,
			config: `
{
  // Useful comments
  "url": "https://github.com",
  "repositoryQuery": ["none"],
  "token": "def",
  "authorization": {}
}`,
			want: `
{
  // Useful comments
  "url": "https://github.com",
  "repositoryQuery": ["none"],
  "token": "def",
  "authorization": {}
}`,
		},
		{
			name: "github without authorization",
			kind: extsvc.KindGitHub,
			config: `
{
  // Useful comments
  "url": "https://github.com",
  "repositoryQuery": ["none"],
  "token": "def"
}`,
			want: `
{
  // Useful comments
  "url": "https://github.com",
  "repositoryQuery": ["none"],
  "token": "def",
  "authorization": {}
}`,
		},
		{
			name: "gitlab with authorization",
			kind: extsvc.KindGitLab,
			config: `
{
  // Useful comments
  "url": "https://gitlab.com",
  "projectQuery": ["none"],
  "token": "abc",
  "authorization": {}
}`,
			want: `
{
  // Useful comments
  "url": "https://gitlab.com",
  "projectQuery": ["none"],
  "token": "abc",
  "authorization": {
    "identityProvider": {
      "type": "oauth"
    }
  }
}`,
		},
		{
			name: "gitlab without authorization",
			kind: extsvc.KindGitLab,
			config: `
{
  // Useful comments
  "url": "https://gitlab.com",
  "projectQuery": ["none"],
  "token": "abc"
}`,
			want: `
{
  // Useful comments
  "url": "https://gitlab.com",
  "projectQuery": ["none"],
  "token": "abc",
  "authorization": {
    "identityProvider": {
      "type": "oauth"
    }
  }
}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := upsertAuthorizationToExternalService(test.kind, test.config)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Fatalf("Mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// This test ensures under Sourcegraph.com mode, every call of `Create`,
// `Upsert` and `Update` has the "authorization" field presented in the external
// service config automatically.
func TestExternalServicesStore_upsertAuthorizationToExternalService(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	db := dbtest.NewDB(t)
	ctx := context.Background()

	envvar.MockSourcegraphDotComMode(true)
	defer envvar.MockSourcegraphDotComMode(false)

	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	externalServices := ExternalServices(db)

	// Test Create method
	es := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
	}
	err := externalServices.Create(ctx, confGet, es)
	require.NoError(t, err)

	got, err := externalServices.GetByID(ctx, es.ID)
	require.NoError(t, err)
	exists := gjson.Get(got.Config, "authorization").Exists()
	assert.True(t, exists, `"authorization" field exists`)

	// Reset Config field and test Upsert method
	es.Config = `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`
	err = externalServices.Upsert(ctx, es)
	require.NoError(t, err)

	got, err = externalServices.GetByID(ctx, es.ID)
	require.NoError(t, err)
	exists = gjson.Get(got.Config, "authorization").Exists()
	assert.True(t, exists, `"authorization" field exists`)

	// Reset Config field and test Update method
	es.Config = `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`
	err = externalServices.Update(ctx,
		conf.Get().AuthProviders,
		es.ID,
		&ExternalServiceUpdate{
			Config: &es.Config,
		},
	)
	require.NoError(t, err)

	got, err = externalServices.GetByID(ctx, es.ID)
	require.NoError(t, err)
	exists = gjson.Get(got.Config, "authorization").Exists()
	assert.True(t, exists, `"authorization" field exists`)
}

func TestCountRepoCount(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := actor.WithInternalActor(context.Background())

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	es1 := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
	}
	err := ExternalServices(db).Create(ctx, confGet, es1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.ExecContext(ctx, `
INSERT INTO repo (id, name, description, fork)
VALUES (1, 'github.com/user/repo', '', FALSE);
`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert rows to `external_service_repos` table to test the trigger.
	q := sqlf.Sprintf(`
INSERT INTO external_service_repos (external_service_id, repo_id, clone_url)
VALUES (%d, 1, '')
`, es1.ID)
	_, err = db.ExecContext(ctx, q.Query(sqlf.PostgresBindVar), q.Args()...)
	if err != nil {
		t.Fatal(err)
	}

	count, err := ExternalServices(db).RepoCount(ctx, es1.ID)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf("Expected 1, got %d", count)
	}
}

func TestExternalServicesStore_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := actor.WithInternalActor(context.Background())

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	es1 := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
	}
	err := ExternalServices(db).Create(ctx, confGet, es1)
	if err != nil {
		t.Fatal(err)
	}

	es2 := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #2",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "def"}`,
	}
	err = ExternalServices(db).Create(ctx, confGet, es2)
	if err != nil {
		t.Fatal(err)
	}

	// Create two repositories to test trigger of soft-deleting external service:
	//  - ID=1 is expected to be deleted along with deletion of the external service.
	//  - ID=2 remains untouched because it is not associated with the external service.
	_, err = db.ExecContext(ctx, `
INSERT INTO repo (id, name, description, fork)
VALUES (1, 'github.com/user/repo', '', FALSE);
INSERT INTO repo (id, name, description, fork)
VALUES (2, 'github.com/user/repo2', '', FALSE);
`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert rows to `external_service_repos` table to test the trigger.
	q := sqlf.Sprintf(`
INSERT INTO external_service_repos (external_service_id, repo_id, clone_url)
VALUES (%d, 1, ''), (%d, 2, '')
`, es1.ID, es2.ID)
	_, err = db.ExecContext(ctx, q.Query(sqlf.PostgresBindVar), q.Args()...)
	if err != nil {
		t.Fatal(err)
	}

	// Delete this external service
	err = ExternalServices(db).Delete(ctx, es1.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Delete again should get externalServiceNotFoundError
	err = ExternalServices(db).Delete(ctx, es1.ID)
	gotErr := fmt.Sprintf("%v", err)
	wantErr := fmt.Sprintf("external service not found: %v", es1.ID)
	if gotErr != wantErr {
		t.Errorf("error: want %q but got %q", wantErr, gotErr)
	}

	_, err = ExternalServices(db).GetByID(ctx, es1.ID)
	if err == nil {
		t.Fatal("expected an error")
	}

	// Should only get back the repo with ID=2
	repos, err := Repos(db).GetByIDs(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	want := []*types.Repo{
		{ID: 2, Name: "github.com/user/repo2"},
	}

	repos = types.Repos(repos).With(func(r *types.Repo) {
		r.CreatedAt = time.Time{}
		r.Sources = nil
	})
	if diff := cmp.Diff(want, repos); diff != "" {
		t.Fatalf("Repos mismatch (-want +got):\n%s", diff)
	}
}

func TestExternalServicesStore_GetByID(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	es := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
	}
	err := ExternalServices(db).Create(ctx, confGet, es)
	if err != nil {
		t.Fatal(err)
	}

	// Should be able to get back by its ID
	_, err = ExternalServices(db).GetByID(ctx, es.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Delete this external service
	err = ExternalServices(db).Delete(ctx, es.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Should now get externalServiceNotFoundError
	_, err = ExternalServices(db).GetByID(ctx, es.ID)
	gotErr := fmt.Sprintf("%v", err)
	wantErr := fmt.Sprintf("external service not found: %v", es.ID)
	if gotErr != wantErr {
		t.Errorf("error: want %q but got %q", wantErr, gotErr)
	}
}

func TestExternalServicesStore_GetByID_Encrypted(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	es := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
	}

	store := ExternalServices(db).WithEncryptionKey(et.TestKey{})

	err := store.Create(ctx, confGet, es)
	if err != nil {
		t.Fatal(err)
	}

	// create a store with a NoopKey to read the raw encrypted value
	noopStore := ExternalServices(db).WithEncryptionKey(&encryption.NoopKey{})
	encrypted, err := noopStore.GetByID(ctx, es.ID)
	if err != nil {
		t.Fatal(err)
	}
	// if the TestKey worked, the config should just be a base64 encoded version
	if encrypted.Config != base64.StdEncoding.EncodeToString([]byte(es.Config)) {
		t.Fatalf("expected base64 encoded config, got %s", encrypted.Config)
	}

	// Should be able to get back by its ID
	_, err = store.GetByID(ctx, es.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Delete this external service
	err = store.Delete(ctx, es.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Should now get externalServiceNotFoundError
	_, err = store.GetByID(ctx, es.ID)
	gotErr := fmt.Sprintf("%v", err)
	wantErr := fmt.Sprintf("external service not found: %v", es.ID)
	if gotErr != wantErr {
		t.Errorf("error: want %q but got %q", wantErr, gotErr)
	}
}

func TestGetAffiliatedSyncErrors(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}

	// Initial user always gets created as an admin
	admin, err := Users(db).Create(ctx, NewUser{
		Email:                 "a1@example.com",
		Username:              "u1",
		Password:              "p",
		EmailVerificationCode: "c",
	})
	if err != nil {
		t.Fatal(err)
	}
	user2, err := Users(db).Create(ctx, NewUser{
		Email:                 "u2@example.com",
		Username:              "u2",
		Password:              "p",
		EmailVerificationCode: "c",
	})
	if err != nil {
		t.Fatal(err)
	}

	createService := func(u *types.User, name string) *types.ExternalService {
		svc := &types.ExternalService{
			Kind:        extsvc.KindGitHub,
			DisplayName: name,
			Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
		}
		if u != nil {
			svc.NamespaceUserID = u.ID
		}
		err = ExternalServices(db).Create(ctx, confGet, svc)
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}

	countErrors := func(results map[int64]string) int {
		var errorCount int
		for _, v := range results {
			if len(v) > 0 {
				errorCount++
			}
		}
		return errorCount
	}

	siteLevel := createService(nil, "GITHUB #1")
	adminOwned := createService(admin, "GITHUB #2")
	userOwned := createService(user2, "GITHUB #3")

	// Listing errors now should return an empty map as none have been added yet
	results, err := ExternalServices(db).GetAffiliatedSyncErrors(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	errorCount := countErrors(results)
	if errorCount != 0 {
		t.Fatal("Expected 0 errors")
	}

	// Add two failures for the same service
	failure1 := "oops"
	_, err = db.Exec(`
INSERT INTO external_service_sync_jobs (external_service_id, state, finished_at, failure_message)
VALUES ($1,'errored', now(), $2)
`, siteLevel.ID, failure1)
	if err != nil {
		t.Fatal(err)
	}
	failure2 := "oops again"
	_, err = db.Exec(`
INSERT INTO external_service_sync_jobs (external_service_id, state, finished_at, failure_message)
VALUES ($1,'errored', now(), $2)
`, siteLevel.ID, failure2)
	if err != nil {
		t.Fatal(err)
	}

	// We should get the latest failure
	results, err = ExternalServices(db).GetAffiliatedSyncErrors(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	errorCount = countErrors(results)
	if errorCount != 1 {
		t.Fatal("Expected 1 error")
	}
	failure := results[siteLevel.ID]
	if failure != failure2 {
		t.Fatalf("Want %q, got %q", failure2, failure)
	}

	// Adding a second failing service
	_, err = db.Exec(`
INSERT INTO external_service_sync_jobs (external_service_id, state, finished_at, failure_message)
VALUES ($1,'errored', now(), $2)
`, adminOwned.ID, failure1)
	if err != nil {
		t.Fatal(err)
	}

	results, err = ExternalServices(db).GetAffiliatedSyncErrors(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatal("Expected 2 results")
	}
	errorCount = countErrors(results)
	if errorCount != 2 {
		t.Fatal("Expected 2 errors")
	}

	// User should not see any failures as they don't own any services
	results, err = ExternalServices(db).GetAffiliatedSyncErrors(ctx, user2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("Expected 1 result")
	}
	errorCount = countErrors(results)
	if errorCount != 0 {
		t.Fatal("Expected 0 errors")
	}

	// Add a failure to user service
	failure3 := "user failure"
	_, err = db.Exec(`
INSERT INTO external_service_sync_jobs (external_service_id, state, finished_at, failure_message)
VALUES ($1,'errored', now(), $2)
`, userOwned.ID, failure3)
	if err != nil {
		t.Fatal(err)
	}

	// We should get the latest failure
	results, err = ExternalServices(db).GetAffiliatedSyncErrors(ctx, user2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("Expected 1 result")
	}
	errorCount = countErrors(results)
	if errorCount != 1 {
		t.Fatal("Expected 1 error")
	}
	failure = results[userOwned.ID]
	if failure != failure3 {
		t.Fatalf("Want %q, got %q", failure3, failure)
	}
}

func TestGetLastSyncError(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	es := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
	}
	err := ExternalServices(db).Create(ctx, confGet, es)
	if err != nil {
		t.Fatal(err)
	}

	// Should be able to get back by its ID
	_, err = ExternalServices(db).GetByID(ctx, es.ID)
	if err != nil {
		t.Fatal(err)
	}

	lastSyncError, err := ExternalServices(db).GetLastSyncError(ctx, es.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lastSyncError != "" {
		t.Fatalf("Expected empty error, have %q", lastSyncError)
	}

	// Could have failure message
	_, err = db.Exec(`
INSERT INTO external_service_sync_jobs (external_service_id, state, finished_at)
VALUES ($1,'errored', now())
`, es.ID)

	if err != nil {
		t.Fatal(err)
	}

	lastSyncError, err = ExternalServices(db).GetLastSyncError(ctx, es.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lastSyncError != "" {
		t.Fatalf("Expected empty error, have %q", lastSyncError)
	}

	// Add sync error
	expectedError := "oops"
	_, err = db.Exec(`
INSERT INTO external_service_sync_jobs (external_service_id, failure_message, state, finished_at)
VALUES ($1,$2,'errored', now())
`, es.ID, expectedError)

	if err != nil {
		t.Fatal(err)
	}

	lastSyncError, err = ExternalServices(db).GetLastSyncError(ctx, es.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lastSyncError != expectedError {
		t.Fatalf("Expected %q, have %q", expectedError, lastSyncError)
	}
}

func TestExternalServicesStore_List(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	// Create test user
	user, err := Users(db).Create(ctx, NewUser{
		Email:           "alice@example.com",
		Username:        "alice",
		Password:        "password",
		EmailIsVerified: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create test org
	displayName := "Acme Org"
	org, err := Orgs(db).Create(ctx, "acme", &displayName)
	if err != nil {
		t.Fatal(err)
	}

	// Create new external services
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	ess := []*types.ExternalService{
		{
			Kind:            extsvc.KindGitHub,
			DisplayName:     "GITHUB #1",
			Config:          `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc", "authorization": {}}`,
			NamespaceUserID: user.ID,
		},
		{
			Kind:        extsvc.KindGitHub,
			DisplayName: "GITHUB #2",
			Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "def"}`,
		},
		{
			Kind:           extsvc.KindGitHub,
			DisplayName:    "GITHUB #3",
			Config:         `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "def", "authorization": {}}`,
			NamespaceOrgID: org.ID,
		},
	}
	for _, es := range ess {
		err := ExternalServices(db).Create(ctx, confGet, es)
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("list all external services", func(t *testing.T) {
		got, err := ExternalServices(db).List(ctx, ExternalServicesListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })

		if diff := cmp.Diff(ess, got); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list all external services in ascending order", func(t *testing.T) {
		got, err := ExternalServices(db).List(ctx, ExternalServicesListOptions{OrderByDirection: "ASC"})
		if err != nil {
			t.Fatal(err)
		}
		want := []*types.ExternalService(types.ExternalServices(ess).Clone())
		sort.Slice(want, func(i, j int) bool { return want[i].ID < want[j].ID })

		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list all external services in descending order", func(t *testing.T) {
		got, err := ExternalServices(db).List(ctx, ExternalServicesListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		want := []*types.ExternalService(types.ExternalServices(ess).Clone())
		sort.Slice(want, func(i, j int) bool { return want[i].ID > want[j].ID })

		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list external services with certain IDs", func(t *testing.T) {
		got, err := ExternalServices(db).List(ctx, ExternalServicesListOptions{
			IDs: []int64{ess[1].ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })

		if diff := cmp.Diff(ess[1:2], got); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list external services with no namespace", func(t *testing.T) {
		got, err := ExternalServices(db).List(ctx, ExternalServicesListOptions{
			NoNamespace: true,
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != 1 {
			t.Fatalf("Want 1 external service but got %d", len(ess))
		} else if diff := cmp.Diff(ess[1], got[0]); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list only test user's external services", func(t *testing.T) {
		got, err := ExternalServices(db).List(ctx, ExternalServicesListOptions{
			NamespaceUserID: user.ID,
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != 1 {
			t.Fatalf("Want 1 external service but got %d", len(ess))
		} else if diff := cmp.Diff(ess[0], got[0]); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list non-exist user's external services", func(t *testing.T) {
		ess, err := ExternalServices(db).List(ctx, ExternalServicesListOptions{
			NamespaceUserID: 404,
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(ess) != 0 {
			t.Fatalf("Want 0 external service but got %d", len(ess))
		}
	})

	t.Run("list only test org's external services", func(t *testing.T) {
		got, err := ExternalServices(db).List(ctx, ExternalServicesListOptions{
			NamespaceOrgID: org.ID,
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != 1 {
			t.Fatalf("Want 1 external service but got %d", len(ess))
		} else if diff := cmp.Diff(ess[2], got[0]); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list non-existing org external services", func(t *testing.T) {
		ess, err := ExternalServices(db).List(ctx, ExternalServicesListOptions{
			NamespaceOrgID: 404,
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(ess) != 0 {
			t.Fatalf("Want 0 external service but got %d", len(ess))
		}
	})
}

func TestExternalServicesStore_DistinctKinds(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	t.Run("no external service won't blow up", func(t *testing.T) {
		kinds, err := ExternalServices(db).DistinctKinds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(kinds) != 0 {
			t.Fatalf("Kinds: want 0 but got %d", len(kinds))
		}
	})

	// Create new external services in different kinds
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	ess := []*types.ExternalService{
		{
			Kind:        extsvc.KindGitHub,
			DisplayName: "GITHUB #1",
			Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
		},
		{
			Kind:        extsvc.KindGitHub,
			DisplayName: "GITHUB #2",
			Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "def"}`,
		},
		{
			Kind:        extsvc.KindGitLab,
			DisplayName: "GITLAB #1",
			Config:      `{"url": "https://github.com", "projectQuery": ["none"], "token": "abc"}`,
		},
		{
			Kind:        extsvc.KindOther,
			DisplayName: "OTHER #1",
			Config:      `{"repos": []}`,
		},
	}
	for _, es := range ess {
		err := ExternalServices(db).Create(ctx, confGet, es)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Delete the last external service which should be excluded from the result
	err := ExternalServices(db).Delete(ctx, ess[3].ID)
	if err != nil {
		t.Fatal(err)
	}

	kinds, err := ExternalServices(db).DistinctKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(kinds)
	wantKinds := []string{extsvc.KindGitHub, extsvc.KindGitLab}
	if diff := cmp.Diff(wantKinds, kinds); diff != "" {
		t.Fatalf("Kinds mismatch (-want +got):\n%s", diff)
	}
}

func TestExternalServicesStore_Count(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	es := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
	}
	err := ExternalServices(db).Create(ctx, confGet, es)
	if err != nil {
		t.Fatal(err)
	}

	count, err := ExternalServices(db).Count(ctx, ExternalServicesListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf("Want 1 external service but got %d", count)
	}
}

func TestExternalServicesStore_Upsert(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	clock := timeutil.NewFakeClock(time.Now(), 0)

	svcs := typestest.MakeExternalServices()

	t.Run("no external services", func(t *testing.T) {
		if err := ExternalServices(db).Upsert(ctx); err != nil {
			t.Fatalf("Upsert error: %s", err)
		}
	})

	t.Run("one external service", func(t *testing.T) {
		tx, err := ExternalServices(db).Transact(ctx)
		if err != nil {
			t.Fatalf("Transact error: %s", err)
		}
		defer func() {
			err = tx.Done(err)
			if err != nil {
				t.Fatalf("Done error: %s", err)
			}
		}()

		svc := svcs[1]
		if svc.Kind != extsvc.KindGitLab {
			t.Fatalf("expected external service at [1] to be GitLab; got %s", svc.Kind)
		}

		if err := tx.Upsert(ctx, svc); err != nil {
			t.Fatalf("upsert error: %v", err)
		}
		if *svc.HasWebhooks != false {
			t.Fatalf("unexpected HasWebhooks: %v", svc.HasWebhooks)
		}

		// Add webhooks to the config and upsert.
		svc.Config = `{"webhooks":[{"secret": "secret"}],` + svc.Config[1:]
		if err := tx.Upsert(ctx, svc); err != nil {
			t.Fatalf("upsert error: %v", err)
		}
		if *svc.HasWebhooks != true {
			t.Fatalf("unexpected HasWebhooks: %v", svc.HasWebhooks)
		}
	})

	t.Run("many external services", func(t *testing.T) {
		user, err := Users(db).Create(ctx, NewUser{Username: "alice"})
		if err != nil {
			t.Fatalf("Test setup error %s", err)
		}
		org, err := Orgs(db).Create(ctx, "acme", nil)
		if err != nil {
			t.Fatalf("Test setup error %s", err)
		}

		namespacedSvcs := typestest.MakeNamespacedExternalServices(user.ID, org.ID)

		tx, err := ExternalServices(db).Transact(ctx)
		if err != nil {
			t.Fatalf("Transact error: %s", err)
		}
		defer func() {
			err = tx.Done(err)
			if err != nil {
				t.Fatalf("Done error: %s", err)
			}
		}()

		services := append(svcs, namespacedSvcs...)
		want := typestest.GenerateExternalServices(11, services...)

		if err := tx.Upsert(ctx, want...); err != nil {
			t.Fatalf("Upsert error: %s", err)
		}

		for _, e := range want {
			if e.Kind != strings.ToUpper(e.Kind) {
				t.Errorf("external service kind didn't get upper-cased: %q", e.Kind)
				break
			}
		}

		sort.Sort(want)

		have, err := tx.List(ctx, ExternalServicesListOptions{
			Kinds: services.Kinds(),
		})
		if err != nil {
			t.Fatalf("List error: %s", err)
		}

		sort.Sort(types.ExternalServices(have))

		if diff := cmp.Diff(have, []*types.ExternalService(want), cmpopts.EquateEmpty()); diff != "" {
			t.Fatalf("List:\n%s", diff)
		}

		// We'll update the external services, but being careful to keep the
		// config valid as we go.
		now := clock.Now()
		suffix := "-updated"
		for _, r := range want {
			r.DisplayName += suffix
			r.Config = `{"wanted":true,` + r.Config[1:]
			r.UpdatedAt = now
			r.CreatedAt = now
		}

		if err = tx.Upsert(ctx, want...); err != nil {
			t.Errorf("Upsert error: %s", err)
		}
		have, err = tx.List(ctx, ExternalServicesListOptions{})
		if err != nil {
			t.Fatalf("List error: %s", err)
		}

		sort.Sort(types.ExternalServices(have))

		if diff := cmp.Diff(have, []*types.ExternalService(want), cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("List:\n%s", diff)
		}

		// Delete external services
		for _, es := range want {
			if err := tx.Delete(ctx, es.ID); err != nil {
				t.Fatal(err)
			}
		}

		have, err = tx.List(ctx, ExternalServicesListOptions{})
		if err != nil {
			t.Errorf("List error: %s", err)
		}

		sort.Sort(types.ExternalServices(have))

		if diff := cmp.Diff(have, []*types.ExternalService(nil), cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("List:\n%s", diff)
		}
	})

	t.Run("with encryption key", func(t *testing.T) {
		tx, err := ExternalServices(db).WithEncryptionKey(et.TestKey{}).Transact(ctx)
		if err != nil {
			t.Fatalf("Transact error: %s", err)
		}
		defer func() {
			err = tx.Done(err)
			if err != nil {
				t.Fatalf("Done error: %s", err)
			}
		}()

		want := typestest.GenerateExternalServices(7, svcs...)

		if err := tx.Upsert(ctx, want...); err != nil {
			t.Fatalf("Upsert error: %s", err)
		}

		// create a store with a NoopKey to read the raw encrypted value
		noopStore := ExternalServicesWith(tx).WithEncryptionKey(&encryption.NoopKey{})

		for _, e := range want {
			if e.Kind != strings.ToUpper(e.Kind) {
				t.Errorf("external service kind didn't get upper-cased: %q", e.Kind)
				break
			}
			encrypted, err := noopStore.GetByID(ctx, e.ID)
			if err != nil {
				t.Fatal(err)
			}
			// if the TestKey worked, the config should just be a base64 encoded version
			if encrypted.Config != base64.StdEncoding.EncodeToString([]byte(e.Config)) {
				t.Fatalf("expected base64 encoded config, got %s", encrypted.Config)
			}
		}

		sort.Sort(want)

		have, err := tx.List(ctx, ExternalServicesListOptions{
			Kinds: svcs.Kinds(),
		})
		if err != nil {
			t.Fatalf("List error: %s", err)
		}

		sort.Sort(types.ExternalServices(have))

		if diff := cmp.Diff(have, []*types.ExternalService(want), cmpopts.EquateEmpty()); diff != "" {
			t.Fatalf("List:\n%s", diff)
		}

		// We'll update the external services, but being careful to keep the
		// config valid as we go.
		now := clock.Now()
		suffix := "-updated"
		for _, r := range want {
			r.DisplayName += suffix
			r.Config = `{"wanted":true,` + r.Config[1:]
			r.UpdatedAt = now
			r.CreatedAt = now
		}

		if err = tx.Upsert(ctx, want...); err != nil {
			t.Errorf("Upsert error: %s", err)
		}
		have, err = tx.List(ctx, ExternalServicesListOptions{})
		if err != nil {
			t.Fatalf("List error: %s", err)
		}

		sort.Sort(types.ExternalServices(have))

		if diff := cmp.Diff(have, []*types.ExternalService(want), cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("List:\n%s", diff)
		}

		// Delete external services
		for _, es := range want {
			if err := tx.Delete(ctx, es.ID); err != nil {
				t.Fatal(err)
			}
		}

		have, err = tx.List(ctx, ExternalServicesListOptions{})
		if err != nil {
			t.Errorf("List error: %s", err)
		}

		sort.Sort(types.ExternalServices(have))

		if diff := cmp.Diff(have, []*types.ExternalService(nil), cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("List:\n%s", diff)
		}
	})
}

func TestExternalServiceStore_GetExternalServiceSyncJobs(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	// Create a new external service
	confGet := func() *conf.Unified {
		return &conf.Unified{}
	}
	es := &types.ExternalService{
		Kind:        extsvc.KindGitHub,
		DisplayName: "GITHUB #1",
		Config:      `{"url": "https://github.com", "repositoryQuery": ["none"], "token": "abc"}`,
	}
	err := ExternalServices(db).Create(ctx, confGet, es)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.ExecContext(ctx, "INSERT INTO external_service_sync_jobs (external_service_id) VALUES ($1)", es.ID)
	if err != nil {
		t.Fatal(err)
	}

	have, err := ExternalServices(db).GetSyncJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(have) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(have))
	}

	want := &types.ExternalServiceSyncJob{
		ID:                1,
		State:             "queued",
		ExternalServiceID: es.ID,
	}
	if diff := cmp.Diff(want, have[0], cmpopts.IgnoreFields(types.ExternalServiceSyncJob{}, "ID")); diff != "" {
		t.Fatal(diff)
	}
}

func TestExternalServicesStore_OneCloudDefaultPerKind(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	now := time.Now()

	makeService := func(cloudDefault bool) *types.ExternalService {
		cfg := `{"url": "https://github.com", "token": "abc", "repositoryQuery": ["none"]}`
		svc := &types.ExternalService{
			Kind:         extsvc.KindGitHub,
			DisplayName:  "Github - Test",
			Config:       cfg,
			CreatedAt:    now,
			UpdatedAt:    now,
			CloudDefault: cloudDefault,
		}
		return svc
	}

	t.Run("non default", func(t *testing.T) {
		gh := makeService(false)
		if err := ExternalServices(db).Upsert(ctx, gh); err != nil {
			t.Fatalf("Upsert error: %s", err)
		}
	})

	t.Run("first default", func(t *testing.T) {
		gh := makeService(true)
		if err := ExternalServices(db).Upsert(ctx, gh); err != nil {
			t.Fatalf("Upsert error: %s", err)
		}
	})

	t.Run("second default", func(t *testing.T) {
		gh := makeService(true)
		if err := ExternalServices(db).Upsert(ctx, gh); err == nil {
			t.Fatal("Expected an error")
		}
	})
}

func TestExternalServiceStore_SyncDue(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	db := dbtest.NewDB(t)
	ctx := context.Background()

	now := time.Now()

	makeService := func() *types.ExternalService {
		cfg := `{"url": "https://github.com", "token": "abc", "repositoryQuery": ["none"]}`
		svc := &types.ExternalService{
			Kind:        extsvc.KindGitHub,
			DisplayName: "Github - Test",
			Config:      cfg,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		return svc
	}
	svc1 := makeService()
	svc2 := makeService()
	err := ExternalServices(db).Upsert(ctx, svc1, svc2)
	if err != nil {
		t.Fatal(err)
	}

	assertDue := func(d time.Duration, want bool) {
		t.Helper()
		ids := []int64{svc1.ID, svc2.ID}
		due, err := ExternalServices(db).SyncDue(ctx, ids, d)
		if err != nil {
			t.Error(err)
		}
		if due != want {
			t.Errorf("Want %v, got %v", want, due)
		}
	}

	makeSyncJob := func(svcID int64, state string) {
		_, err = db.Exec(`
INSERT INTO external_service_sync_jobs (external_service_id, state)
VALUES ($1,$2)
`, svcID, state)
		if err != nil {
			t.Fatal(err)
		}
	}

	// next_sync_at is null, so we expect a sync soon
	assertDue(1*time.Second, true)

	// next_sync_at in the future
	_, err = db.Exec("UPDATE external_services SET next_sync_at = $1", now.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	assertDue(1*time.Second, false)
	assertDue(11*time.Minute, true)

	// With sync jobs
	makeSyncJob(svc1.ID, "queued")
	makeSyncJob(svc2.ID, "completed")
	assertDue(1*time.Minute, true)

	// Sync jobs not running
	_, err = db.Exec("UPDATE external_service_sync_jobs SET state = 'completed'")
	if err != nil {
		t.Fatal(err)
	}
	assertDue(1*time.Minute, false)
}

func TestConfigurationHasWebhooks(t *testing.T) {
	t.Run("supported kinds with webhooks", func(t *testing.T) {
		for _, cfg := range []interface{}{
			&schema.GitHubConnection{
				Webhooks: []*schema.GitHubWebhook{
					{Org: "org", Secret: "super secret"},
				},
			},
			&schema.GitLabConnection{
				Webhooks: []*schema.GitLabWebhook{
					{Secret: "super secret"},
				},
			},
			&schema.BitbucketServerConnection{
				Plugin: &schema.BitbucketServerPlugin{
					Webhooks: &schema.BitbucketServerPluginWebhooks{
						Secret: "super secret",
					},
				},
			},
		} {
			t.Run(fmt.Sprintf("%T", cfg), func(t *testing.T) {
				assert.True(t, configurationHasWebhooks(cfg))
			})
		}
	})

	t.Run("supported kinds without webhooks", func(t *testing.T) {
		for _, cfg := range []interface{}{
			&schema.GitHubConnection{},
			&schema.GitLabConnection{},
			&schema.BitbucketServerConnection{},
		} {
			t.Run(fmt.Sprintf("%T", cfg), func(t *testing.T) {
				assert.False(t, configurationHasWebhooks(cfg))
			})
		}
	})

	t.Run("unsupported kinds", func(t *testing.T) {
		for _, cfg := range []interface{}{
			&schema.AWSCodeCommitConnection{},
			&schema.BitbucketCloudConnection{},
			&schema.GitoliteConnection{},
			&schema.PerforceConnection{},
			&schema.PhabricatorConnection{},
			&schema.JVMPackagesConnection{},
			&schema.OtherExternalServiceConnection{},
			nil,
		} {
			t.Run(fmt.Sprintf("%T", cfg), func(t *testing.T) {
				assert.False(t, configurationHasWebhooks(cfg))
			})
		}
	})
}
