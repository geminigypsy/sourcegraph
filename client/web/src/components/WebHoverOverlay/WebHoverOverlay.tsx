import React, { useCallback, useEffect } from 'react'
import { fromEvent } from 'rxjs'
import { finalize, tap } from 'rxjs/operators'

import { isErrorLike } from '@sourcegraph/common'
import { urlForClientCommandOpen } from '@sourcegraph/shared/src/actions/ActionItem'
import { NotificationType } from '@sourcegraph/shared/src/api/extension/extensionHostApi'
import { HoverOverlay, HoverOverlayProps } from '@sourcegraph/shared/src/hover/HoverOverlay'
import { AlertProps, useLocalStorage } from '@sourcegraph/wildcard'

import { GlobalCoolCodeIntelProps } from '../../global/CoolCodeIntel'
import { HoverThresholdProps } from '../../repo/RepoContainer'

import styles from './WebHoverOverlay.module.scss'

const iconKindToAlertVariant: Record<number, AlertProps['variant']> = {
    [NotificationType.Info]: 'secondary',
    [NotificationType.Error]: 'danger',
    [NotificationType.Warning]: 'warning',
}

const getAlertVariant: HoverOverlayProps['getAlertVariant'] = iconKind => iconKindToAlertVariant[iconKind]

interface Props extends HoverOverlayProps, HoverThresholdProps, GlobalCoolCodeIntelProps {
    hoveredTokenElement?: HTMLElement
    nav?: (url: string) => void
}

export const WebHoverOverlay: React.FunctionComponent<Props> = props => {
    const [dismissedAlerts, setDismissedAlerts] = useLocalStorage<string[]>('WebHoverOverlay.dismissedAlerts', [])
    const onAlertDismissed = useCallback(
        (alertType: string) => {
            if (!dismissedAlerts.includes(alertType)) {
                setDismissedAlerts([...dismissedAlerts, alertType])
            }
        },
        [dismissedAlerts, setDismissedAlerts]
    )

    let propsToUse = props
    if (props.hoverOrError && props.hoverOrError !== 'loading' && !isErrorLike(props.hoverOrError)) {
        const filteredAlerts = (props.hoverOrError?.alerts || []).filter(
            alert => !alert.type || !dismissedAlerts.includes(alert.type)
        )
        propsToUse = { ...props, hoverOrError: { ...props.hoverOrError, alerts: filteredAlerts } }
    }

    const { hoverOrError } = propsToUse
    const { onHoverShown, hoveredToken, onTokenClick, coolCodeIntelEnabled } = props

    /** Whether the hover has actual content (that provides value to the user) */
    const hoverHasValue = hoverOrError !== 'loading' && !isErrorLike(hoverOrError) && !!hoverOrError?.contents?.length

    useEffect(() => {
        if (hoverHasValue) {
            onHoverShown?.()
        }
    }, [hoveredToken?.filePath, hoveredToken?.line, hoveredToken?.character, onHoverShown, hoverHasValue])

    useEffect(() => {
        const token = props.hoveredTokenElement

        const definitionAction =
            Array.isArray(props.actionsOrError) &&
            props.actionsOrError.find(a => a.action.id === 'goToDefinition.preloaded' && !a.disabledWhen)

        const referenceAction =
            Array.isArray(props.actionsOrError) &&
            props.actionsOrError.find(a => a.action.id === 'findReferences' && !a.disabledWhen)

        const action = definitionAction || referenceAction
        if (!action) {
            return undefined
        }
        const url = urlForClientCommandOpen(action.action, props.location.hash)

        if (!token || !url || !props.nav) {
            return
        }

        const nav = props.nav

        const oldCursor = token.style.cursor
        token.style.cursor = 'pointer'

        const subscription = fromEvent(token, 'click')
            .pipe(
                tap(() => {
                    const selection = window.getSelection()
                    if (selection !== null && selection.toString() !== '') {
                        return
                    }

                    if (coolCodeIntelEnabled && onTokenClick !== undefined && hoveredToken !== undefined) {
                        onTokenClick(hoveredToken)
                    } else {
                        const actionType = action === definitionAction ? 'definition' : 'reference'
                        props.telemetryService.log(`${actionType}HoverOverlay.click`)
                        nav(url)
                    }
                }),
                finalize(() => (token.style.cursor = oldCursor))
            )
            .subscribe()

        return () => subscription.unsubscribe()
    }, [
        props.actionsOrError,
        props.hoveredTokenElement,
        props.location.hash,
        props.nav,
        props.telemetryService,
        hoveredToken,
        coolCodeIntelEnabled,
        onTokenClick,
    ])

    const onlyGoToDefinition = Array.isArray(props.actionsOrError)
        ? props.actionsOrError.filter(
              a => a.action.id === 'goToDefinition.preloaded' || a.action.id === 'goToDefinition'
          )
        : []

    return (
        <HoverOverlay
            {...propsToUse}
            {...(coolCodeIntelEnabled && {
                actionsOrError: onlyGoToDefinition,
            })}
            className={styles.webHoverOverlay}
            actionItemClassName="border-0"
            onAlertDismissed={onAlertDismissed}
            getAlertVariant={getAlertVariant}
            actionItemStyleProps={{
                actionItemSize: 'sm',
                actionItemVariant: 'secondary',
            }}
        />
    )
}

WebHoverOverlay.displayName = 'WebHoverOverlay'
