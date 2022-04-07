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

import React from "react";

const Instructions = () => {
    const doRender = () => {
        return (<>
            <h3>How to Play</h3>
            <div>
                <ol>
                    <li>Install Lift at <a href="https://lift.sonatype.com">lift.sonatype.com</a> (see also: <a href="https://help.sonatype.com/lift/getting-started">Lift - Getting Started</a>)</li>
                    <li>Pick a Project</li>
                    <li>Review the project contributing guidelines</li>
                    <li>Find a Bug in the Bug List</li>
                    <li>Smash the Bug</li>
                    <li>Win Points</li>
                </ol>
            </div>
            <h3>How to Smash a Bug</h3>
            <div>
                <ul>
                    <li><b>Fork Chosen Project and Clone</b></li>
                    <br/>
                    <li><b>Smash Bug in Local Environment</b>
                    <br/>
                    Usually best done with a keyboard, but sometimes a hammer is the only thing to get the job done.</li>
                    <br/>
                    <li><b>IMPORTANT! Create a Pull Request back to Upstream GitHub Repository</b>
                    <br/>
                    This is how you get your points, so don’t forget it!</li>
                </ul>
            </div>
        </>)
    }

    return (
        doRender()
    )

}

export default Instructions;
