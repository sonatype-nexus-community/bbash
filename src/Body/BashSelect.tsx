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
    NxFormGroup,
    NxFormSelect,
    nxFormSelectStateHelpers,
    NxLoadError,
    NxTooltip
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

type BashSelectProps = {
    setCampaign: any
}


const BashSelect = (props: BashSelectProps) => {

    const [queryError, setQueryError] = useState<queryError>({error: false, errorMessage: ""}),
        [bashListFetched, setBashListFetched] = useState(false),
        [bashList, setBashList] = useState<Campaign[]>([]),
        [selectedCampaign, setSelectedCampaign] = useState<Campaign>();

    const clientContext = useContext(ClientContext);
    const getBashList = async () => {
        if (bashListFetched) { // @todo better way to avoid looping?
            return;
        }

        const getBashesAction: Action = {
            method: 'GET',
            endpoint: `/campaign/list`
        }
        const res = await clientContext.query(getBashesAction);

        if (!res.error) {
            setBashListFetched(true); // @todo better way to avoid looping?
            setBashList(res.payload ? res.payload : []);
        } else {
            setQueryError({error: true, errorMessage: res.payload});
        }
    }

    const onChange = (evt: FormEvent<HTMLSelectElement>) => {
        const selectedGuid = evt.currentTarget.value;
        const foundCampaign = bashList.find(element => element.guid == selectedGuid)
        setSelectedCampaign(foundCampaign);
        // props.setCampaign(evt.currentTarget.value);
        props.setCampaign(foundCampaign);
    }

    const doRender = () => {
        getBashList();

        if (queryError.error) {
            return <NxLoadError error={queryError.errorMessage}/>;
        }

        return (
            <NxTooltip title="Select a Bash" placement="top">
                <NxFormGroup label={`Selected Bash: ${selectedCampaign?.name} (${selectedCampaign?.guid}) created on: ${selectedCampaign?.createdOn}`} isRequired>
                    <NxFormSelect onChange={onChange}>
                        {bashList.length ? bashList.map((bash) =>
                            <option value={bash.guid}>{bash.name}</option>)
                            : <option value="0">No Bashes Available</option>}
                    </NxFormSelect>
                </NxFormGroup>
            </NxTooltip>
        )
    }

    return (
        doRender()
    )
}

export default BashSelect;

export type {Campaign}
