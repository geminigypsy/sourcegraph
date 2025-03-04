import { Meta, Story } from '@storybook/react'
import React from 'react'

import { NOOP_TELEMETRY_SERVICE } from '@sourcegraph/shared/src/telemetry/telemetryService'

import { WebStory } from '../../../../../../components/WebStory'
import { CodeInsightsBackendContext } from '../../../../core/backend/code-insights-backend-context'
import { CodeInsightsGqlBackend } from '../../../../core/backend/gql-api/code-insights-gql-backend'

import { IntroCreationPage } from './IntroCreationPage'

export default {
    title: 'web/insights/creation-ui/IntroPage',
    decorators: [story => <WebStory>{() => story()}</WebStory>],
    parameters: {
        chromatic: {
            viewports: [576, 978, 1440],
            disableSnapshot: false,
        },
    },
} as Meta

const API = new CodeInsightsGqlBackend({} as any)

export const IntroPage: Story = () => (
    <CodeInsightsBackendContext.Provider value={API}>
        <IntroCreationPage telemetryService={NOOP_TELEMETRY_SERVICE} />
    </CodeInsightsBackendContext.Provider>
)
