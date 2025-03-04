/* eslint-disable ban/ban */
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as H from 'history'
import React from 'react'
import { MemoryRouter } from 'react-router'
import { Route } from 'react-router-dom'
import { of } from 'rxjs'
import sinon from 'sinon'

import { MockedTestProvider } from '@sourcegraph/shared/src/testing/apollo'
import { MockIntersectionObserver } from '@sourcegraph/shared/src/testing/MockIntersectionObserver'

import { CodeInsightsBackend } from '../core/backend/code-insights-backend'
import {
    CodeInsightsBackendContext,
    FakeDefaultCodeInsightsBackend,
} from '../core/backend/code-insights-backend-context'
import { ALL_INSIGHTS_DASHBOARD_ID } from '../core/types/dashboard/virtual-dashboard'

import { CodeInsightsRootPage, CodeInsightsRootPageTab } from './CodeInsightsRootPage'

interface ReactRouterMock {
    useHistory: () => unknown
    useRouteMatch: () => unknown
}

const url = '/insights'

jest.mock('react-router', () => ({
    ...jest.requireActual<ReactRouterMock>('react-router'),
    useHistory: () => ({
        push: jest.fn(),
    }),
    useRouteMatch: () => ({
        url,
    }),
}))

const mockTelemetryService = {
    log: sinon.spy(),
    logViewEvent: sinon.spy(),
}

const fakeApi = new FakeDefaultCodeInsightsBackend()

const Wrapper: React.FunctionComponent<{ api: Partial<CodeInsightsBackend> }> = ({ children, api = {} }) => {
    const extendedApi: CodeInsightsBackend = {
        ...fakeApi,
        ...api,
    }
    return <CodeInsightsBackendContext.Provider value={extendedApi}>{children}</CodeInsightsBackendContext.Provider>
}

const renderWithBrandedContext = (component: React.ReactElement, { route = '/', api = {} } = {}) => {
    // eslint-disable-next-line @typescript-eslint/ban-ts-comment
    // @ts-ignore
    const routerSettings: { testHistory: H.History; testLocation: H.Location } = {}

    return {
        ...render(
            <MockedTestProvider>
                <Wrapper api={api}>
                    <MemoryRouter initialEntries={[route]}>
                        {component}
                        <Route
                            path="*"
                            render={({ history, location }) => {
                                routerSettings.testHistory = history
                                routerSettings.testLocation = location
                                return null
                            }}
                        />
                    </MemoryRouter>
                </Wrapper>
            </MockedTestProvider>
        ),
        ...routerSettings,
    }
}

describe('CodeInsightsRootPage', () => {
    beforeAll(() => {
        window.IntersectionObserver = MockIntersectionObserver
    })

    it('should redirect to "All insights" page if no dashboardId is provided', () => {
        const { testLocation } = renderWithBrandedContext(
            <CodeInsightsRootPage
                activeView={CodeInsightsRootPageTab.CodeInsights}
                telemetryService={mockTelemetryService}
            />,
            {
                route: '/insights/dashboards/',
                api: {
                    isCodeInsightsLicensed: () => of(true),
                },
            }
        )

        expect(testLocation.pathname).toEqual(`${url}/${ALL_INSIGHTS_DASHBOARD_ID}`)
    })

    it('should render dashboard not found page when id is not found', () => {
        renderWithBrandedContext(
            <CodeInsightsRootPage
                activeView={CodeInsightsRootPageTab.CodeInsights}
                telemetryService={mockTelemetryService}
            />,
            {
                route: '/insights/dashboards/foo',
                api: {
                    getDashboardSubjects: () => of([]),
                    getDashboards: () => of([]),
                    isCodeInsightsLicensed: () => of(true),
                },
            }
        )

        screen.findByText("Hmm, the dashboard wasn't found.")
    })

    it('should log events', () => {
        renderWithBrandedContext(
            <CodeInsightsRootPage
                activeView={CodeInsightsRootPageTab.CodeInsights}
                telemetryService={mockTelemetryService}
            />,
            {
                route: '/insights/dashboards/foo',
                api: {
                    getDashboardSubjects: () => of([]),
                    getDashboards: () => of([]),
                    isCodeInsightsLicensed: () => of(true),
                },
            }
        )

        expect(mockTelemetryService.logViewEvent.calledWith('Insights')).toBe(true)

        userEvent.click(screen.getByText('Create insight'))
        expect(mockTelemetryService.log.calledWith('InsightAddMoreClick')).toBe(true)
    })
})
