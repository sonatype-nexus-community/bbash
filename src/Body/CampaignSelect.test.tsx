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
import {render} from '@testing-library/react';
import React from 'react';
import {ClientContextProvider, createClient} from 'react-fetching-library';

import CampaignSelect, {qp} from './CampaignSelect';
import {MockResponseObject} from "fetch-mock";
import fetchMock from "fetch-mock-jest";

describe("<CampaignSelect></CampaignSelect>", () => {
    test("Should have no campaign selected by default", async () => {
        const client = createClient({});

        const {findByText} = render(
            <ClientContextProvider client={client}>
                <CampaignSelect setSelectedCampaign={() => {}}/>
            </ClientContextProvider>
        );

        expect(await findByText("Select a campaign")).toBeTruthy()
    });

    test("Should show error if failure reading campaign list", async () => {
        let myError = new Error("forced fetch error");
        let mockResponse: MockResponseObject = {
            throws: myError,
        }
        fetchMock.get(`/campaign/active?${qp.feature}=activeCampaigns&${qp.call}=useEffect`,
            mockResponse
        );

        const client = createClient({});
        const {findByText} = render(
            <ClientContextProvider client={client}>
                <CampaignSelect setSelectedCampaign={() => {}}/>
            </ClientContextProvider>
        );

        expect(await findByText((content) => {
            return content === "An error occurred loading data. " + myError.toString()
        })).toBeTruthy()
    });

});

