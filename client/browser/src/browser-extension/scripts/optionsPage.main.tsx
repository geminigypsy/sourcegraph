// We want to polyfill first.
import '../../shared/polyfills'

import { trimEnd, uniq } from 'lodash'
import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { render } from 'react-dom'
import { from, noop, Observable } from 'rxjs'
import { catchError, distinctUntilChanged, map, mapTo } from 'rxjs/operators'
import { Optional } from 'utility-types'

import { asError } from '@sourcegraph/common'
import { GraphQLResult } from '@sourcegraph/http-client'
import { TelemetryService } from '@sourcegraph/shared/src/telemetry/telemetryService'
import { setLinkComponent, AnchorLink, useObservable } from '@sourcegraph/wildcard'

import { fetchSite } from '../../shared/backend/server'
import { WildcardThemeProvider } from '../../shared/components/WildcardThemeProvider'
import { initSentry } from '../../shared/sentry'
import { ConditionalTelemetryService, EventLogger } from '../../shared/tracking/eventLogger'
import { observeSourcegraphURL, getExtensionVersion, isDefaultSourcegraphUrl } from '../../shared/util/context'
import { featureFlags } from '../../shared/util/featureFlags'
import {
    OptionFlagKey,
    optionFlagDefinitions,
    observeSendTelemetry,
    observeOptionFlagsWithValues,
} from '../../shared/util/optionFlags'
import { assertEnvironment } from '../environmentAssertion'
import { KnownCodeHost, knownCodeHosts } from '../knownCodeHosts'
import { OptionsPage, URL_AUTH_ERROR, URL_FETCH_ERROR } from '../options-menu/OptionsPage'
import { ThemeWrapper } from '../ThemeWrapper'
import { checkUrlPermissions } from '../util'
import { background } from '../web-extension-api/runtime'
import { observeStorageKey, storage } from '../web-extension-api/storage'

interface TabStatus {
    host: string
    protocol: string
    hasPermissions: boolean
    hasPrivateCloudError: boolean
}

assertEnvironment('OPTIONS')

initSentry('options')

const IS_EXTENSION = true

/**
 * A list of protocols where we should *not* show the permissions notification.
 */
const PERMISSIONS_PROTOCOL_BLOCKLIST = new Set(['chrome:', 'about:', 'safari-web-extension:'])

setLinkComponent(AnchorLink)

const isOptionFlagKey = (key: string): key is OptionFlagKey =>
    !!optionFlagDefinitions.find(definition => definition.key === key)

const fetchCurrentTabStatus = async (): Promise<TabStatus> => {
    const tabs = await browser.tabs.query({ active: true, currentWindow: true })
    if (tabs.length > 1) {
        throw new Error('Querying for the currently active tab returned more than one result')
    }
    const { url, id } = tabs[0]
    if (!url) {
        throw new Error('Currently active tab has no URL')
    }
    if (!id) {
        throw new Error('Currently active tab has no ID')
    }
    const hasPrivateCloudError = await background.checkPrivateCloudError(id)
    const { host, protocol } = new URL(url)
    const hasPermissions = await checkUrlPermissions(url)
    return { hasPrivateCloudError, host, protocol, hasPermissions }
}

// Make GraphQL requests from background page
const createRequestGraphQL = (sourcegraphURL: string) => <T, V = object>(options: {
    request: string
    variables: V
}): Observable<GraphQLResult<T>> =>
    from(
        background.requestGraphQL<T, V>({ ...options, sourcegraphURL })
    )

const version = getExtensionVersion()
const isFullPage = !new URLSearchParams(window.location.search).get('popup')

const validateSourcegraphUrl = (url: string): Observable<string | undefined> =>
    fetchSite(options => createRequestGraphQL(url)(options)).pipe(
        mapTo(undefined),
        catchError(error => {
            const { message } = asError(error)
            // We lose Error type when communicating from the background page
            // to the options page, so we determine the error type from the message
            if (message.includes('Failed to fetch')) {
                return [URL_FETCH_ERROR]
            }
            if (message.includes('401')) {
                return [URL_AUTH_ERROR]
            }

            return [message]
        })
    )

const observingIsActivated = observeStorageKey('sync', 'disableExtension').pipe(map(isDisabled => !isDisabled))
const observingPreviouslyUsedUrls = observeStorageKey('sync', 'previouslyUsedURLs')
const observingSourcegraphUrl = observeSourcegraphURL(true).pipe(distinctUntilChanged())
const observingOptionFlagsWithValues = observeOptionFlagsWithValues(IS_EXTENSION)
const observingSendTelemetry = observeSendTelemetry(IS_EXTENSION)

function handleToggleActivated(isActivated: boolean): void {
    storage.sync.set({ disableExtension: !isActivated }).catch(console.error)
}

function handleChangeOptionFlag(key: string, value: boolean): void {
    if (isOptionFlagKey(key)) {
        featureFlags.set(key, value).catch(noop)
    }
}

function buildRequestPermissionsHandler({ protocol, host }: TabStatus) {
    return function requestPermissionsHandler(event: React.MouseEvent) {
        event.preventDefault()
        browser.permissions.request({ origins: [`${protocol}//${host}/*`] }).catch(error => {
            console.error('Error requesting permissions:', error)
        })
    }
}

function useTelemetryService(sourcegraphUrl: string | undefined): TelemetryService {
    const telemetryService = useMemo(
        () =>
            new ConditionalTelemetryService(
                new EventLogger(createRequestGraphQL(sourcegraphUrl!), sourcegraphUrl!),
                observingSendTelemetry
            ),
        [sourcegraphUrl]
    )

    useEffect(() => () => telemetryService.unsubscribe(), [telemetryService])
    return telemetryService
}

/**
 * Returns unique URLs
 */
const uniqURLs = (urls: (string | undefined)[]): string[] =>
    uniq(urls.filter(value => !!value).map(value => trimEnd(value, '/')))

const Options: React.FunctionComponent = () => {
    const sourcegraphUrl = useObservable(observingSourcegraphUrl)
    const [previousSourcegraphUrl, setPreviousSourcegraphUrl] = useState(sourcegraphUrl)
    const telemetryService = useTelemetryService(sourcegraphUrl)
    const previouslyUsedUrls = useObservable(observingPreviouslyUsedUrls)
    const isActivated = useObservable(observingIsActivated)
    const optionFlagsWithValues = useObservable(observingOptionFlagsWithValues)
    const [currentTabStatus, setCurrentTabStatus] = useState<
        { status: TabStatus; handler: React.MouseEventHandler } | undefined
    >()

    useEffect(() => {
        fetchCurrentTabStatus().then(tabStatus => {
            setCurrentTabStatus({ status: tabStatus, handler: buildRequestPermissionsHandler(tabStatus) })
        }, noop)
    }, [])

    const showSourcegraphCloudAlert = currentTabStatus?.status.host.endsWith('sourcegraph.com')

    let permissionAlert: Optional<KnownCodeHost, 'host' | 'icon'> | undefined
    if (
        currentTabStatus &&
        !currentTabStatus?.status.hasPermissions &&
        !showSourcegraphCloudAlert &&
        !PERMISSIONS_PROTOCOL_BLOCKLIST.has(currentTabStatus.status.protocol)
    ) {
        const knownCodeHost = knownCodeHosts.find(({ host }) => host === currentTabStatus.status.host)
        if (knownCodeHost) {
            permissionAlert = knownCodeHost
        } else {
            permissionAlert = { name: currentTabStatus.status.host }
        }
    }

    const handleChangeSourcegraphUrl = useCallback(
        (url: string): void => {
            if (sourcegraphUrl === url) {
                return
            }
            storage.sync
                .set({
                    sourcegraphURL: url,
                    previouslyUsedURLs: uniqURLs([...(previouslyUsedUrls || []), url, sourcegraphUrl]),
                })
                .catch(console.error)
        },
        [previouslyUsedUrls, sourcegraphUrl]
    )

    useEffect(() => {
        setPreviousSourcegraphUrl(sourcegraphUrl)
    }, [sourcegraphUrl])

    useEffect(() => {
        if (
            previousSourcegraphUrl !== sourcegraphUrl &&
            isDefaultSourcegraphUrl(sourcegraphUrl) &&
            previouslyUsedUrls &&
            previouslyUsedUrls.length >= 2
        ) {
            telemetryService.log('Bext_NumberURLs')
        }
    }, [sourcegraphUrl, telemetryService, previouslyUsedUrls, previousSourcegraphUrl])

    return (
        <ThemeWrapper>
            <WildcardThemeProvider isBranded={true}>
                <OptionsPage
                    isFullPage={isFullPage}
                    sourcegraphUrl={sourcegraphUrl || ''}
                    suggestedSourcegraphUrls={uniqURLs(previouslyUsedUrls || [])}
                    onChangeSourcegraphUrl={handleChangeSourcegraphUrl}
                    version={version}
                    validateSourcegraphUrl={validateSourcegraphUrl}
                    isActivated={!!isActivated}
                    onToggleActivated={handleToggleActivated}
                    optionFlags={optionFlagsWithValues || []}
                    onChangeOptionFlag={handleChangeOptionFlag}
                    showPrivateRepositoryAlert={
                        currentTabStatus?.status.hasPrivateCloudError && isDefaultSourcegraphUrl(sourcegraphUrl)
                    }
                    showSourcegraphCloudAlert={showSourcegraphCloudAlert}
                    permissionAlert={permissionAlert}
                    requestPermissionsHandler={currentTabStatus?.handler}
                />
            </WildcardThemeProvider>
        </ThemeWrapper>
    )
}

const inject = (): void => {
    render(<Options />, document.body)
}

document.addEventListener('DOMContentLoaded', inject)
