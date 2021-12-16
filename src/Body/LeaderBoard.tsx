/*
 * Copyright (c) 2021-present Sonatype, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
import {NxLoadError, NxTable} from "@sonatype/react-shared-components"
import React, {useContext, useState} from "react"
import {Campaign} from "./CampaignSelect";
import {Action, ClientContext} from "react-fetching-library";

type BashSelectProps = {
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


const LeaderBoard = (props: BashSelectProps) => {

    const [queryError, setQueryError] = useState<queryError>({error: false, errorMessage: ""}),
        [currentCampaign, setCurrentCampaign] = useState<Campaign>(),
        [participantList, setParticipantList] = useState<Participant[]>();


    const clientContext = useContext(ClientContext);
    const getLeaders = async (campaign: Campaign) => {

        if (participantList?.length && campaign.name == currentCampaign?.name) { // @todo better way to avoid looping?
            return;
        }

        const getLeadersAction: Action = {
            method: 'GET',
            endpoint: `/participant/list/${campaign.name}`
        }
        const res = await clientContext.query(getLeadersAction);

        if (!res.error) {
            setParticipantList(res.payload ? res.payload : []);  // @todo better way to avoid looping?
            setCurrentCampaign(campaign)
        } else {
            setQueryError({error: true, errorMessage: res.payload});
        }
    }

    const doRender = (campaign: Campaign) => {
        getLeaders(campaign);

        if (queryError.error) {
            return <NxLoadError error={queryError.errorMessage}/>;
        }

        return (
            <NxTable>
                <NxTable.Head>
                    <NxTable.Row>
                        <NxTable.Cell>GitHub ID</NxTable.Cell>
                        <NxTable.Cell isNumeric>Points</NxTable.Cell>
                    </NxTable.Row>
                </NxTable.Head>
                <NxTable.Body>
                    {participantList?.length ? participantList.map((participant) =>
                            <NxTable.Row>
                                <NxTable.Cell>{participant.loginName}</NxTable.Cell>
                                <NxTable.Cell isNumeric>{participant.score}</NxTable.Cell>
                            </NxTable.Row>
                        )
                        : <NxTable.Row>No Participants</NxTable.Row>}
                </NxTable.Body>
            </NxTable>
        )
    }

    if (props.selectedCampaign) {
        return doRender(props.selectedCampaign);
    }
    return null;
}

export default LeaderBoard;
