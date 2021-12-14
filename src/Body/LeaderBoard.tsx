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
import {NxTable} from "@sonatype/react-shared-components"
import React, {FormEvent, useContext, useState} from "react"
import {Campaign} from "./BashSelect";
import {Action, ClientContext} from "react-fetching-library";

type BashSelectProps = {
    selectedBash?: Campaign;
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
        [leadersFetched, setLeadersFetched] = useState(false),
        [leadersList, setLeadersList] = useState<Participant[]>([]),
        [priorCampaign, setPriorCampaign] = useState<Campaign>();


    const clientContext = useContext(ClientContext);
    const getLeaders = async (campaign: Campaign) => {
        if (leadersFetched) { // @todo better way to avoid looping?
            return;
        }

        const getLeadersAction: Action = {
            method: 'GET',
            endpoint: `/participant/list/${campaign?.name}`
        }
        const res = await clientContext.query(getLeadersAction);

        if (!res.error) {
            setLeadersFetched(true); // @todo better way to avoid looping?

            console.log("leaders fetched")
            console.log(res.payload)

            setLeadersList(res.payload ? res.payload : []);
        } else {
            setQueryError({error: true, errorMessage: res.payload});
        }
    }

    const onChange = (evt: FormEvent<HTMLSelectElement>) => {
        console.log("leader changes")
    }

    let lastCampaign: Campaign
    const doRender = (campaign: Campaign) => {
        console.log("render leaders, campaign %s, prior: %s, last: %s", campaign.name, priorCampaign?.name, lastCampaign?.name)
        // @todo How to trigger refresh of participants w/out forever loop
        // if (priorCampaign && campaign && priorCampaign.guid != campaign.guid) {
        //     setLeadersFetched(false);
        // }
        getLeaders(campaign);
        //setPriorCampaign(campaign)
        lastCampaign = campaign

        return (
            <NxTable>
                <NxTable.Head>
                    <NxTable.Row>
                        <NxTable.Cell>GitHub ID</NxTable.Cell>
                        <NxTable.Cell isNumeric>Points</NxTable.Cell>
                    </NxTable.Row>
                </NxTable.Head>
                <NxTable.Body>
                    {leadersList.length ? leadersList.map((participant) =>
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

    if (props.selectedBash) {
        return doRender(props.selectedBash);
    }
    return null;
}

export default LeaderBoard;
