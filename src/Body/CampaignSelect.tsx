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
import {
    NxFormSelect,
    NxLoadError
} from "@sonatype/react-shared-components"
import React, {FormEvent, useContext, useState} from "react"
import {Action, ClientContext} from "react-fetching-library";

interface Campaign {
    guid: string
    name: string
    createdOn: string
}

type queryError = {
    error: boolean
    errorMessage: string
}

type CampaignSelectProps = {
    selectedCampaign: any
}


const CampaignSelect = (props: CampaignSelectProps) => {

    const [queryError, setQueryError] = useState<queryError>({error: false, errorMessage: ""}),
        [campaignList, setCampaignList] = useState<Campaign[]>([]);

    const clientContext = useContext(ClientContext);

    const getCampaignList = async () => {
        if (campaignList?.length || queryError?.error) { // @todo better way to avoid looping?
            return
        }

        const getCampaignsAction: Action = {
            method: 'GET',
            endpoint: `/campaign/list`
        }
        const res = await clientContext.query(getCampaignsAction);

        if (!res.error) {
            setCampaignList(res.payload ? res.payload : []); // @todo better way to avoid looping?
        } else {
            setQueryError({error: true, errorMessage: res.payload});
        }
    }

    const onChange = (evt: FormEvent<HTMLSelectElement>) => {
        const selectedGuid = evt.currentTarget.value;
        const foundCampaign = campaignList.find(element => element.guid === selectedGuid)
        props.selectedCampaign(foundCampaign);
    }

    const doRender = () => {
        getCampaignList();

        if (queryError.error) {
            return <NxLoadError error={queryError?.errorMessage} data-testid="campaign-select-error"/>;
        }

        return (
            <NxFormSelect onChange={onChange} data-testid="campaign-select">
                {campaignList.length ? campaignList.map((bash) =>
                        <option value={bash.guid}>{bash.name}</option>)
                    : <option value="0">No Campaigns Available</option>}
            </NxFormSelect>
        )
    }

    return (
        doRender()
    )
}

export default CampaignSelect;

export type {Campaign}
