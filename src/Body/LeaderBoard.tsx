/*
 * Copyright (c) 2021-present Sonatype, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
import {NxButton, NxLoadError, NxTable} from "@sonatype/react-shared-components"
import React, {MouseEvent, useCallback, useContext, useEffect, useState} from "react"
import {Campaign} from "./CampaignSelect";
import {Action, ClientContext} from "react-fetching-library";

type CampaignSelectProps = {
    selectedCampaign?: Campaign;
}

interface Participant {
    guid: string
    campaignName: string
    scpName: string
    loginName: string
    displayName: string
    email: string
    score: number
    team: string
    joinedAt: string
}

type queryError = {
    error: boolean
    errorMessage: string
}

const qpFeature = "feature"
const qpCaller = "caller"

const LeaderBoard = (props: CampaignSelectProps) => {

    const [queryError, setQueryError] = useState<queryError>({error: false, errorMessage: ""}),
        [participantList, setParticipantList] = useState<Participant[]>();

    const clientContext = useContext(ClientContext);

    const getLeaders = useCallback(async (campaign: Campaign | undefined, caller) => {
        if (!campaign) {
            console.debug("no selectedCampaign, skipping getLeaders")
            return
        }

        const getLeadersAction: Action = {
            method: 'GET',
            endpoint: `/participant/list/${campaign.name}?${qpFeature}=getLeaders&${qpCaller}=${caller}`
        }
        const res = await clientContext.query(getLeadersAction);

        if (!res.error) {
            setParticipantList(res.payload ? res.payload : []);
        } else {
            const errMsg = (res && res.payload) ? res.payload.error : res.errorObject.toString()
            setQueryError({error: true, errorMessage: errMsg});
        }
    }, [clientContext])

    useEffect(() => {
        // noinspection JSIgnoredPromiseFromCall
        getLeaders(props.selectedCampaign, "useEffect");
    }, [clientContext, props.selectedCampaign, getLeaders]) // rebuilds the list only when selectedCampaign changes

    // noinspection JSUnusedLocalSymbols
    const onClick = (evt: MouseEvent<HTMLButtonElement>) => {
        // noinspection JSIgnoredPromiseFromCall
        getLeaders(props.selectedCampaign, "refreshScores");
    }

    const doRender = () => {
        if (queryError.error) {
            return <NxLoadError error={queryError.errorMessage}/>;
        }

        return (
            <>
                <NxButton variant="secondary" onClick={onClick}>Refresh Scores</NxButton>

                <NxTable>
                    <NxTable.Head>
                        <NxTable.Row>
                            <NxTable.Cell>Source Code Repository User Name</NxTable.Cell>
                            <NxTable.Cell isNumeric>Score</NxTable.Cell>
                        </NxTable.Row>
                    </NxTable.Head>
                    <NxTable.Body>
                        {participantList?.length ? participantList.map((participant) =>
                                <NxTable.Row>
                                    <NxTable.Cell>{participant.loginName}</NxTable.Cell>
                                    <NxTable.Cell isNumeric>{participant.score}</NxTable.Cell>
                                </NxTable.Row>
                            )
                            : <NxTable.Row>
                                <NxTable.Cell>No Participants</NxTable.Cell>
                                <NxTable.Cell isNumeric> </NxTable.Cell>
                            </NxTable.Row>}
                    </NxTable.Body>
                </NxTable>
            </>
        )
    }

    if (props.selectedCampaign) {
        return doRender();
    }
    return null;
}

export default LeaderBoard;
