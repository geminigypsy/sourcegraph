// Package usagestats provides an interface to update and access information about
// individual and aggregate Sourcegraph users' activity levels.
package usagestats

import (
	"context"

	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/types"
)

func GetGrowthStatistics(ctx context.Context, db database.DB) (*types.GrowthStatistics, error) {
	// TODO: Fix query
	const q = `
-- source: internal/usagestats/growth.go:GetGrowthStatistics
WITH all_usage_by_user_and_month AS (
    SELECT user_id,
           DATE_TRUNC('month', timestamp) AS month_active
      FROM event_logs
     GROUP BY user_id,
              month_active),
recent_usage_by_user AS (
    SELECT users.id,
           BOOL_OR(CASE WHEN DATE_TRUNC('month', month_active) = DATE_TRUNC('month', now()) THEN TRUE ELSE FALSE END) AS current_month,
           BOOL_OR(CASE WHEN DATE_TRUNC('month', month_active) = DATE_TRUNC('month', now()) - INTERVAL '1 month' THEN TRUE ELSE FALSE END) AS previous_month,
           DATE_TRUNC('month', DATE(users.created_at)) AS created_month,
           DATE_TRUNC('month', DATE(users.deleted_at)) AS deleted_month
      FROM users
      LEFT JOIN all_usage_by_user_and_month ON all_usage_by_user_and_month.user_id = users.id
     GROUP BY id,
              created_month,
              deleted_month)
SELECT COUNT(*) FILTER ( WHERE recent_usage_by_user.created_month = DATE_TRUNC('month', now())) AS created_users,
       COUNT(*) FILTER ( WHERE recent_usage_by_user.deleted_month = DATE_TRUNC('month', now())) AS deleted_users,
       COUNT(*) FILTER (
                 WHERE current_month = TRUE
                   AND previous_month = FALSE
                   AND created_month < DATE_TRUNC('month', now())
                   AND (deleted_month < DATE_TRUNC('month', now()) OR deleted_month IS NULL)) AS resurrected_users,
       COUNT(*) FILTER (
                 WHERE current_month = FALSE
                   AND previous_month = TRUE
                   AND created_month < DATE_TRUNC('month', now())
                   AND (deleted_month < DATE_TRUNC('month', now()) OR deleted_month IS NULL)) AS churned_users,
       COUNT(*) FILTER (
                 WHERE current_month = TRUE
                   AND previous_month = TRUE
                   AND created_month < DATE_TRUNC('month', now())
                   AND (deleted_month < DATE_TRUNC('month', now()) OR deleted_month IS NULL)) AS retained_users
  FROM recent_usage_by_user
    `
	var (
		createdUsers     int
		deletedUsers     int
		resurrectedUsers int
		churnedUsers     int
		retainedUsers    int
	)
	if err := db.QueryRowContext(ctx, q).Scan(
		&createdUsers,
		&deletedUsers,
		&resurrectedUsers,
		&churnedUsers,
		&retainedUsers,
	); err != nil {
		return nil, err
	}

	return &types.GrowthStatistics{
		DeletedUsers:     int32(deletedUsers),
		CreatedUsers:     int32(createdUsers),
		ResurrectedUsers: int32(resurrectedUsers),
		ChurnedUsers:     int32(churnedUsers),
		RetainedUsers:    int32(retainedUsers),
	}, nil
}

func GetCTAUsage(ctx context.Context, db database.DB) (*types.CTAUsage, error) {
	const query = `
 -- source: internal/usagestats/growth.go:GetCTAUsage
 WITH data_by_month AS (
     SELECT name,
            user_id,
            DATE_TRUNC('month', timestamp) AS month,
            argument->>'page' AS page
       FROM event_logs
      WHERE name IN ('InstallBrowserExtensionCTAShown', 'InstallBrowserExtensionCTAClicked' )
        AND argument->>'page' IN ('file', 'search')
        AND DATE_TRUNC('month', timestamp) = DATE_TRUNC('month', $1::timestamp)
 )
 WITH data_by_month_and_user AS (
     SELECT name,
            user_id,
            DATE_TRUNC('month', timestamp) AS month,
            argument->>'page' AS page
       FROM event_logs
      WHERE name IN ('InstallBrowserExtensionCTAShown', 'InstallBrowserExtensionCTAClicked' )
        AND argument->>'page' IN ('file', 'search')
        AND DATE_TRUNC('month', timestamp) = DATE_TRUNC('month', $1::timestamp)
      GROUP BY user_id
 )
 SELECT COUNT(*) FILTER (
                  WHERE name = 'InstallBrowserExtensionCTAShown'
                    AND page = 'file') AS user_count_who_saw_bext_cta_on_file_page,
        COUNT(*) FILTER (
                  WHERE name = 'InstallBrowserExtensionCTAClicked'
                    AND page = 'file') AS user_count_who_clicked_bext_cta_on_file_page,
        COUNT(*) FILTER (
                  WHERE name = 'InstallBrowserExtensionCTAShown'
                    AND page = 'search') AS user_count_who_saw_bext_cta_on_search_page,
        COUNT(*) FILTER (
                  WHERE name = 'InstallBrowserExtensionCTAClicked'
                    AND page = 'search') AS user_count_who_clicked_bext_cta_on_search_page
   FROM data_by_month_and_user,
        COUNT(*) FILTER (
                  WHERE name = 'InstallBrowserExtensionCTAShown'
                    AND page = 'file') AS bext_cta_displays_on_file_page,
        COUNT(*) FILTER (
                  WHERE name = 'InstallBrowserExtensionCTAClicked'
                    AND page = 'file') AS bext_cta_clicks_on_file_page,
        COUNT(*) FILTER (
                  WHERE name = 'InstallBrowserExtensionCTAShown'
                    AND page = 'search') AS bext_cta_displays_on_search_page,
        COUNT(*) FILTER (
                  WHERE name = 'InstallBrowserExtensionCTAClicked'
                    AND page = 'search') AS bext_cta_clicks_on_search_page
   FROM data_by_month
`
	var (
		userCountWhoSawBextCtaOnFilePage       int32
		userCountWhoClickedBextCtaOnFilePage   int32
		userCountWhoSawBextCtaOnSearchPage     int32
		userCountWhoClickedBextCtaOnSearchPage int32
		bextCtaDisplaysOnFilePage              int32
		bextCtaClicksOnFilePage                int32
		bextCtaDisplaysOnSearchPage            int32
		bextCtaClicksOnSearchPage              int32
	)
	if err := db.QueryRowContext(ctx, query, timeNow()).Scan(
		&userCountWhoSawBextCtaOnFilePage,
		&userCountWhoClickedBextCtaOnFilePage,
		&userCountWhoSawBextCtaOnSearchPage,
		&userCountWhoClickedBextCtaOnSearchPage,
		&bextCtaDisplaysOnFilePage,
		&bextCtaClicksOnFilePage,
		&bextCtaDisplaysOnSearchPage,
		&bextCtaClicksOnSearchPage,
	); err != nil {
		return nil, err
	}

	return &types.CTAUsage{
		UserCountWhoSawBextCtaOnFilePage:       userCountWhoSawBextCtaOnFilePage,
		UserCountWhoClickedBextCtaOnFilePage:   userCountWhoClickedBextCtaOnFilePage,
		UserCountWhoSawBextCtaOnSearchPage:     userCountWhoSawBextCtaOnSearchPage,
		UserCountWhoClickedBextCtaOnSearchPage: userCountWhoClickedBextCtaOnSearchPage,
		BextCtaDisplaysOnFilePage:              bextCtaDisplaysOnFilePage,
		BextCtaClicksOnFilePage:                bextCtaClicksOnFilePage,
		BextCtaDisplaysOnSearchPage:            bextCtaDisplaysOnSearchPage,
		BextCtaClicksOnSearchPage:              bextCtaClicksOnSearchPage,
	}, nil
}
