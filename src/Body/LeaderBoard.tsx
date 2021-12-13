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
import React from "react"
import {Campaign} from "./BashSelect";

type BashSelectProps = {
    selectedBash?: Campaign;
}

const LeaderBoard = (props: BashSelectProps) => {

    const doRender = (campaign: Campaign) => {
        return (
            <NxTable>
                <NxTable.Head>
                    <NxTable.Row>
                        <NxTable.Cell>GitHub ID</NxTable.Cell>
                        <NxTable.Cell isNumeric>Points</NxTable.Cell>
                    </NxTable.Row>
                </NxTable.Head>
                <NxTable.Body>
                    <NxTable.Row>
                        <NxTable.Cell>Content 1</NxTable.Cell>
                        <NxTable.Cell isNumeric>4</NxTable.Cell>
                    </NxTable.Row>
                    <NxTable.Row>
                        <NxTable.Cell>Content 1</NxTable.Cell>
                        <NxTable.Cell isNumeric>4</NxTable.Cell>
                    </NxTable.Row>
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
