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
import {NxFormGroup, NxFormSelect, nxFormSelectStateHelpers} from "@sonatype/react-shared-components"
import React, {FormEvent} from "react"

type Campaign = {
    id: string
    name: string
    createdOn: string
}

var selectedCampaign: Campaign

const BashSelect = () => {
    const [selectState, setSelectValue] = nxFormSelectStateHelpers.useNxFormSelectState<number>(1);

    function onChange(evt: FormEvent<HTMLSelectElement>) {
        setSelectValue(parseInt(evt.currentTarget.value));
    }

    return (
        <NxFormGroup label={`Selected Option: ${selectState.value}`} isRequired>
            <NxFormSelect>
                <option value="1">Option 1</option>
            </NxFormSelect>
        </NxFormGroup>
    )
}

export default BashSelect;
