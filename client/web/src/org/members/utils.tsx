import classNames from 'classnames'
import CloseIcon from 'mdi-react/CloseIcon'
import React from 'react'

import { Alert, Button } from '@sourcegraph/wildcard'

import styles from './InviteMemberModal.module.scss'

export const getInvitationExpiryDateString = (expiring: string): string => {
    const expiryDate = new Date(expiring)
    const now = new Date().getTime()
    const diff = expiryDate.getTime() - now
    const numberOfDays = diff / (1000 * 3600 * 24)
    if (numberOfDays < 1) {
        return 'today'
    }

    const numberDaysInt = Math.round(numberOfDays)

    if (numberDaysInt === 1) {
        return 'tomorrow'
    }

    return `in ${numberDaysInt} days`
}

export const getInvitationCreationDateString = (creation: string): string => {
    const creationDate = new Date(creation)
    const now = new Date().getTime()
    const diff = now - creationDate.getTime()
    const numberOfDays = diff / (1000 * 3600 * 24)
    const numberDaysInt = Math.round(numberOfDays)
    if (numberDaysInt < 1) {
        return 'today'
    }

    if (numberDaysInt === 1) {
        return 'yesterday'
    }

    return `${numberDaysInt} days ago`
}

export const getLocaleFormattedDateFromString = (jsonDate: string): string => new Date(jsonDate).toLocaleDateString()

interface MembersNotificationProps {
    message: string
    onDismiss: () => void
    className?: string
}

export const OrgMemberNotification: React.FunctionComponent<MembersNotificationProps> = ({
    className,
    message,
    onDismiss,
}) => (
    <Alert variant="success" className={classNames(styles.invitedNotification, className)}>
        <div className={styles.message}>{message}</div>
        <Button className="btn-icon" title="Dismiss" onClick={onDismiss}>
            <CloseIcon className="icon-inline" />
        </Button>
    </Alert>
)
