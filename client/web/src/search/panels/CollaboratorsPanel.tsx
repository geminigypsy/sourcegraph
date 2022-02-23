import classNames from 'classnames'
import EmailCheckIcon from 'mdi-react/EmailCheckIcon'
import EmailIcon from 'mdi-react/EmailIcon'
import React, { useCallback, useMemo, useState } from 'react'
import { Observable } from 'rxjs'

import { ErrorAlert } from '@sourcegraph/branded/src/components/alerts'
import { ErrorLike, isErrorLike } from '@sourcegraph/common'
import { TelemetryProps } from '@sourcegraph/shared/src/telemetry/telemetryService'
import { Button, LoadingSpinner, useObservable } from '@sourcegraph/wildcard'

import { AuthenticatedUser } from '../../auth'
import { InvitableCollaborator } from '../../auth/welcome/InviteCollaborators/InviteCollaborators'
import { UserAvatar } from '../../user/UserAvatar'

import styles from './CollaboratorsPanel.module.scss'
import { LoadingPanelView } from './LoadingPanelView'
import { PanelContainer } from './PanelContainer'

interface Props extends TelemetryProps {
    className?: string
    authenticatedUser: AuthenticatedUser | null
    fetchCollaborators: (userId: string) => Observable<InvitableCollaborator[]>
}

export const CollaboratorsPanel: React.FunctionComponent<Props> = ({
    className,
    authenticatedUser,
    fetchCollaborators,
    telemetryService,
}) => {
    const collaborators = useObservable(
        useMemo(() => fetchCollaborators(authenticatedUser?.id || ''), [fetchCollaborators, authenticatedUser?.id])
    )
    const filteredCollaborators = useMemo(() => collaborators?.slice(0, 6), [collaborators])

    const [inviteError, setInviteError] = useState<ErrorLike | null>(null)
    const [loadingInvites, setLoadingInvites] = useState<Set<string>>(new Set<string>())
    const [successfulInvites, setSuccessfulInvites] = useState<Set<string>>(new Set<string>())
    const invitePerson = useCallback(
        async (person: InvitableCollaborator): Promise<void> => {
            if (loadingInvites.has(person.email) || successfulInvites.has(person.email)) {
                return
            }
            setLoadingInvites(set => new Set(set).add(person.email))

            try {
                // await inviteEmailToSourcegraph({ variables: { email: person.email } })
                await new Promise(resolve => setTimeout(resolve, 2000))

                setLoadingInvites(set => {
                    const removed = new Set(set)
                    removed.delete(person.email)
                    return removed
                })
                setSuccessfulInvites(set => new Set(set).add(person.email))

                // eventLogger.log('UserInvitationsSentEmailInvite')
            } catch (error) {
                setInviteError(error)
            }
        },
        [loadingInvites, successfulInvites]
    )

    const loadingDisplay = <LoadingPanelView text="Loading colleagues" />

    const contentDisplay = (
        <div className={classNames('row', 'py-1')}>
            {isErrorLike(inviteError) && <ErrorAlert error={inviteError} />}

            {filteredCollaborators?.map((person: InvitableCollaborator) => (
                <div
                    className={classNames('d-flex', 'align-items-center', 'col-lg-6', 'mt-1', 'mb-1', styles.invitebox)}
                    key={person.email}
                >
                    <Button
                        variant="icon"
                        key={person.email}
                        disabled={loadingInvites.has(person.email) || successfulInvites.has(person.email)}
                        className={classNames('w-100', styles.button)}
                        onClick={() => invitePerson(person)}
                    >
                        <UserAvatar size={40} className={classNames(styles.avatar, 'mr-3')} user={person} />
                        <div className={styles.content}>
                            <strong className={styles.clipText}>{person.displayName}dasdfjlaskdfjlsakfd</strong>
                            <div className={styles.inviteButton}>
                                {loadingInvites.has(person.email) ? (
                                    <span className=" ml-auto mr-3">
                                        <LoadingSpinner inline={true} className="icon-inline mr-1" />
                                        Inviting...
                                    </span>
                                ) : successfulInvites.has(person.email) ? (
                                    <span className="text-success ml-auto mr-3">
                                        <EmailCheckIcon className="icon-inline mr-1" />
                                        Invited
                                    </span>
                                ) : (
                                    <>
                                        <div className={classNames('text-muted', styles.clipText)}>{person.email}</div>
                                        <div className={classNames('text-primary', styles.inviteButtonOverlay)}>
                                            <EmailIcon className="icon-inline mr-1" />
                                            Invite to Sourcegraph
                                        </div>
                                    </>
                                )}
                            </div>
                        </div>
                    </Button>
                </div>
            ))}
        </div>
    )

    return (
        <PanelContainer
            className={classNames(className, 'h-100')}
            title="Invite your colleagues"
            hideTitle={true}
            state={collaborators === undefined ? 'loading' : 'populated'}
            loadingContent={loadingDisplay}
            populatedContent={contentDisplay}
        />
    )
}
