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
import {render} from '@testing-library/react';
import React from 'react';
import {ClientContextProvider, createClient} from 'react-fetching-library';

import LeaderBoard from './LeaderBoard';
import {Campaign} from "./CampaignSelect";

describe("<LeaderBoard></LeaderBoard>", () => {
    test("Should have no participants shown by default", async () => {
        const client = createClient({});

        const selectedCampaign: Campaign = {
            guid: "myCampaignGuid",
            name: "myCampaignName",
            createdOn: Date()
        };

        const {findByText} = render(
            <ClientContextProvider client={client}>
                <LeaderBoard selectedCampaign={selectedCampaign}/>
            </ClientContextProvider>
        );

        const foundElement = await findByText("No Participants");
        expect(foundElement).toBeTruthy()
    });
});
