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
import React, {useState} from "react";
import CampaignSelect, {Campaign} from "./CampaignSelect";
import LeaderBoard from "./LeaderBoard";

const Body = () => {

    const [selectedCampaign, setSelectedCampaign] = useState<Campaign>()

    const doRender = () => {

      return <>
          <h1>Welcome to the Bug Bash!</h1>
          <CampaignSelect selectedCampaign={setSelectedCampaign}/>
          <LeaderBoard selectedCampaign={selectedCampaign}/>
        </>
    }

    return (
      doRender()
    )
}

export default Body;