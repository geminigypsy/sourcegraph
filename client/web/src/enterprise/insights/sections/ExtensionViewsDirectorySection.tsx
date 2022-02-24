import { useApolloClient } from '@apollo/client'
import React, { useContext, useMemo } from 'react'
import { EMPTY, from } from 'rxjs'
import { map, switchMap } from 'rxjs/operators'

import { wrapRemoteObservable } from '@sourcegraph/shared/src/api/client/api/common'
import { ViewProviderResult } from '@sourcegraph/shared/src/api/extension/extensionHostApi'
import { useObservable } from '@sourcegraph/wildcard'

import { ExtensionViewsSectionCommonProps } from '../../../insights/sections/types'
import { StaticView, ViewGrid } from '../../../views'
import { SmartInsight } from '../components/insights-view-grid/components/smart-insight/SmartInsight'
import { CodeInsightsBackendContext } from '../core/backend/code-insights-backend-context'
import { CodeInsightsGqlBackend } from '../core/backend/gql-api/code-insights-gql-backend'
import { Insight } from '../core/types'
import { ALL_INSIGHTS_DASHBOARD_ID } from '../core/types/dashboard/virtual-dashboard'

export interface ExtensionViewsDirectorySectionProps extends ExtensionViewsSectionCommonProps {
    where: 'directory'
    uri: string
}

const EMPTY_EXTENSION_LIST: ViewProviderResult[] = []

/**
 * Renders extension views section for the directory page. Note that this component is used only for
 * Enterprise version. For Sourcegraph OSS see `./src/insights/sections` components.
 */
export const ExtensionViewsDirectorySection: React.FunctionComponent<ExtensionViewsDirectorySectionProps> = props => {
    const { settingsCascade, extensionsController, uri, telemetryService, className = '' } = props

    const apolloClient = useApolloClient()

    const api = useMemo(() => new CodeInsightsGqlBackend(apolloClient), [apolloClient])

    return (
        <CodeInsightsBackendContext.Provider value={api}>
            <ExtensionViewsDirectorySectionContent
                where="directory"
                uri={uri}
                extensionsController={extensionsController}
                settingsCascade={settingsCascade}
                telemetryService={telemetryService}
                className={className}
            />
        </CodeInsightsBackendContext.Provider>
    )
}

const EMPTY_INSIGHT_LIST: Insight[] = []

const ExtensionViewsDirectorySectionContent: React.FunctionComponent<ExtensionViewsDirectorySectionProps> = props => {
    const { extensionsController, uri, className } = props

    const { getInsights } = useContext(CodeInsightsBackendContext)

    const workspaceUri = useObservable(
        useMemo(
            () =>
                from(extensionsController.extHostAPI).pipe(
                    switchMap(extensionHostAPI => wrapRemoteObservable(extensionHostAPI.getWorkspaceRoots())),
                    map(workspaceRoots => workspaceRoots[0]?.uri)
                ),
            [extensionsController]
        )
    )
    const directoryPageContext = useMemo(
        () =>
            workspaceUri && {
                viewer: {
                    type: 'DirectoryViewer' as const,
                    directory: {
                        uri: new URL(uri),
                    },
                },
                workspace: {
                    uri: new URL(workspaceUri),
                },
            },
        [uri, workspaceUri]
    )

    // Read code insights views from the settings cascade
    const insights =
        useObservable(useMemo(() => getInsights({ dashboardId: ALL_INSIGHTS_DASHBOARD_ID }), [getInsights])) ??
        EMPTY_INSIGHT_LIST

    // Pull extension views with Extension API
    const extensionViews =
        useObservable(
            useMemo(
                () =>
                    workspaceUri
                        ? from(extensionsController.extHostAPI).pipe(
                              switchMap(extensionHostAPI =>
                                  wrapRemoteObservable(
                                      extensionHostAPI.getDirectoryViews({
                                          viewer: {
                                              type: 'DirectoryViewer',
                                              directory: { uri },
                                          },
                                          workspace: { uri: workspaceUri },
                                      })
                                  )
                              )
                          )
                        : EMPTY,
                [workspaceUri, uri, extensionsController]
            )
        ) ?? EMPTY_EXTENSION_LIST

    const allViewIds = useMemo(() => [...extensionViews, ...insights].map(view => view.id), [extensionViews, insights])

    if (!directoryPageContext) {
        return null
    }

    return (
        <ViewGrid viewIds={allViewIds} className={className}>
            {/* Render extension views for the directory page */}
            {extensionViews.map(view => (
                <StaticView key={view.id} content={view} telemetryService={props.telemetryService} />
            ))}
            {/* Render all code insights with proper directory page context */}
            {insights.map(insight => (
                <SmartInsight
                    key={insight.id}
                    insight={insight}
                    telemetryService={props.telemetryService}
                    where="directory"
                    context={directoryPageContext}
                />
            ))}
        </ViewGrid>
    )
}
