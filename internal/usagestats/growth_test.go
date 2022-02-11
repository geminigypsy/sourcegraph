package usagestats

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/database/dbtest"
	"github.com/sourcegraph/sourcegraph/internal/types"
)

func TestCTAUsageUsageStatistics(t *testing.T) {
	ctx := context.Background()

	defer func() {
		timeNow = time.Now
	}()

	now := time.Date(2021, 1, 20, 12, 55, 0, 0, time.UTC)
	mockTimeNow(now)

	db := database.NewDB(dbtest.NewDB(t))

	_, err := db.ExecContext(context.Background(), `
		INSERT INTO event_logs
			(id, name, argument, url, user_id, anonymous_user_id, source, version, timestamp)
		VALUES
			-- Current day event logs
			-- user_id=1
			(1, 'InstallBrowserExtensionCTAShown', '{"page": "file"}', 'https://sourcegraph.test:3443/search', 1, '420657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 hour'),
			(2, 'InstallBrowserExtensionCTAClicked', '{"page": "file"}', 'https://sourcegraph.test:3443/search', 1, '420657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 hour'),
			(3, 'InstallBrowserExtensionCTAShown', '{"page": "search"}', 'https://sourcegraph.test:3443/search', 1, '420657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 hour'),
			(4, 'InstallBrowserExtensionCTAShown', '{"page": "search"}', 'https://sourcegraph.test:3443/search', 1, '420657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 hour'),
			(5, 'InstallBrowserExtensionCTAClicked', '{"page": "search"}', 'https://sourcegraph.test:3443/search', 1, '420657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 hour'),

			-- user_id=0, anonymous user
			(6, 'InstallBrowserExtensionCTAShown', '{"page": "file"}', 'https://sourcegraph.test:3443/search', 0, '560657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 hour'),
			(7, 'InstallBrowserExtensionCTAClicked', '{"page": "file"}', 'https://sourcegraph.test:3443/search', 0, '560657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 hour'),
			(8, 'InstallBrowserExtensionCTAShown', '{"page": "search"}', 'https://sourcegraph.test:3443/search', 0, '560657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 hour'),
			(9, 'InstallBrowserExtensionCTAShown', '{"page": "search"}', 'https://sourcegraph.test:3443/search', 0, '560657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 hour'),


			-- Previous day event logs
			(10, 'InstallBrowserExtensionCTAShown', '{"page": "file"}', 'https://sourcegraph.test:3443/search', 1, '420657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 day'),
			(11, 'InstallBrowserExtensionCTAClicked', '{"page": "file"}', 'https://sourcegraph.test:3443/search', 1, '420657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 day'),
			(12, 'InstallBrowserExtensionCTAShown', '{"page": "search"}', 'https://sourcegraph.test:3443/search', 1, '420657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 day'),
			(13, 'InstallBrowserExtensionCTAClicked', '{"page": "search"}', 'https://sourcegraph.test:3443/search', 1, '420657f0-d443-4d16-ac7d-003d8cdc91ef', 'WEB', '3.23.0', $1::timestamp - interval '1 day')
	`, now)
	if err != nil {
		t.Fatal(err)
	}

	have, err := GetCTAUsage(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	want := &types.CTAUsage{
		DailyBrowserExtensionCTA: types.FileAndSearchPageUserAndEventCounts{
			StartTime:             time.Date(2021, 1, 20, 0, 0, 0, 0, time.UTC),
			DisplayedOnFilePage:   types.UserAndEventCount{UserCount: 2, EventCount: 2},
			DisplayedOnSearchPage: types.UserAndEventCount{UserCount: 2, EventCount: 4},
			ClickedOnFilePage:     types.UserAndEventCount{UserCount: 2, EventCount: 2},
			ClickedOnSearchPage:   types.UserAndEventCount{UserCount: 1, EventCount: 1},
		},
	}
	if diff := cmp.Diff(want, have); diff != "" {
		t.Fatal(diff)
	}
}
