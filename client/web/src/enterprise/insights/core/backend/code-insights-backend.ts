import { Observable } from 'rxjs'
import { LineChartContent, PieChartContent } from 'sourcegraph'

import { ViewContexts, ViewProviderResult } from '@sourcegraph/shared/src/api/extension/extensionHostApi'

import { BackendInsight, Insight, InsightDashboard } from '../types'
import { SupportedInsightSubject } from '../types/subjects'

import {
    BackendInsightData,
    CaptureInsightSettings,
    DashboardCreateInput,
    DashboardCreateResult,
    DashboardDeleteInput,
    DashboardUpdateInput,
    DashboardUpdateResult,
    FindInsightByNameInput,
    GetBuiltInsightInput,
    GetLangStatsInsightContentInput,
    GetSearchInsightContentInput,
    InsightCreateInput,
    InsightUpdateInput,
    ReachableInsight,
    RepositorySuggestionData,
} from './code-insights-backend-types'

/**
 * The main interface for code insights backend. Each backend versions should
 * implement this interface in order to support all functionality that code insights
 * pages and components have.
 */
export interface CodeInsightsBackend {
    /**
     * Returns all accessible code insights dashboards for the current user.
     * This includes virtual (like "all insights") and real dashboards.
     */
    getDashboards: () => Observable<InsightDashboard[]>

    getDashboardById: (input: { dashboardId: string | undefined }) => Observable<InsightDashboard | null>

    /**
     * Returns all possible visibility options for dashboard. Dashboard can be stored
     * as private (user subject), org level (organization subject) or global (site subject)
     */
    getDashboardSubjects: () => Observable<SupportedInsightSubject[]>

    findDashboardByName: (name: string) => Observable<InsightDashboard | null>

    createDashboard: (input: DashboardCreateInput) => Observable<DashboardCreateResult>

    updateDashboard: (input: DashboardUpdateInput) => Observable<DashboardUpdateResult>

    deleteDashboard: (input: DashboardDeleteInput) => Observable<void>

    assignInsightsToDashboard: (input: DashboardUpdateInput) => Observable<unknown>

    /**
     * Return all accessible for a user insights that are filtered by ids param.
     * If ids is nullable value then returns all insights. Insights in this case
     * present only insight configurations and meta data without actual data about
     * data series or pie chart data.
     *
     * @param ids - list of insight ids
     */
    getInsights: (input: { dashboardId: string }) => Observable<Insight[]>

    /**
     * Returns all reachable subject's insights from subject with subjectId.
     *
     * User subject has access to all insights from all organizations and global site settings.
     * Organization subject has access to only its insights.
     */
    getReachableInsights: (input: { subjectId: string }) => Observable<ReachableInsight[]>

    /**
     * Return insight (meta and presentation data) by insight id.
     * Note that insight model doesn't contain any data series points.
     */
    getInsightById: (id: string) => Observable<Insight | null>

    findInsightByName: (input: FindInsightByNameInput) => Observable<Insight | null>

    hasInsights: () => Observable<boolean>

    createInsight: (input: InsightCreateInput) => Observable<unknown>

    updateInsight: (event: InsightUpdateInput) => Observable<unknown>

    deleteInsight: (insightId: string) => Observable<unknown>

    /**
     * Returns all available for users subjects (sharing levels, historically it was introduced
     * from the setting cascade subject levels - global, org levels, personal)
     */
    getInsightSubjects: () => Observable<SupportedInsightSubject[]>

    /**
     * Returns backend insight (via gql API handler)
     */
    getBackendInsightData: (insight: BackendInsight) => Observable<BackendInsightData>

    /**
     * Returns extension like built-in insight that is fetched via frontend
     * network requests to Sourcegraph search API.
     */
    getBuiltInInsightData: <D extends keyof ViewContexts>(
        input: GetBuiltInsightInput<D>
    ) => Observable<ViewProviderResult>

    /**
     * Returns content for the search based insight live preview chart.
     */
    getSearchInsightContent: <D extends keyof ViewContexts>(
        input: GetSearchInsightContentInput<D>
    ) => Promise<LineChartContent<any, string>>

    /**
     * Returns content for the code stats insight live preview chart.
     */
    getLangStatsInsightContent: <D extends keyof ViewContexts>(
        input: GetLangStatsInsightContentInput<D>
    ) => Promise<PieChartContent<any>>

    getCaptureInsightContent: (input: CaptureInsightSettings) => Promise<LineChartContent<any, string>>

    /**
     * Returns a list of suggestions for the repositories field in the insight creation UI.
     *
     * @param query - A string with a possible value for the repository name
     */
    getRepositorySuggestions: (query: string) => Promise<RepositorySuggestionData[]>

    /**
     * Returns a list of resolved repositories from the search page query via search API.
     * Used by 1-click insight creation flow. Since users can have a repo: filter in their
     * query we have to resolve these filters by our search API.
     *
     * @param query - search page query value
     */
    getResolvedSearchRepositories: (query: string) => Promise<string[]>

    /**
     * Used for the dynamic insight example on the insights landing page.
     * Attempts to return a repoository that contains the string "TODO"
     * If a repository is not found it then returns the first repository it finds.
     *
     * Under the hood this is calling the search API with "select:repo TODO count:1"
     * or "select:repo count:1" if no repository is found with the string "TODO"
     */
    getFirstExampleRepository: () => Observable<string>

    /*
     * Returns whether Code Insights is licensed
     */
    isCodeInsightsLicensed: () => Observable<boolean>
}
