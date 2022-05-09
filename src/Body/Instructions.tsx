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
                    <li>Project Admin: (once) Install Lift at <a href="https://lift.sonatype.com">lift.sonatype.com</a> (see also: <a href="https://help.sonatype.com/lift/getting-started">Lift - Getting Started</a>).</li>
                    <li>Bug Bash Participant: <a href="https://docs.google.com/forms/d/15HWMmI7OMxIamOD2c_rbAts1Jnry5_Tg7wgLRYJK0j4/edit?usp=sharing">Register here</a>.</li>
                    <li>Review the project contributing guidelines ("How To Contribute" links in <a href="./BugLists.html">Bug Lists</a>).</li>
                    <li>Find a Bug in the analysis produced by Lift. See <a href="./BugLists.html">Bug Lists</a>.</li>
                    <li>Smash the Bug with a <a href="https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/creating-a-pull-request">Pull Request</a>.</li>
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
                    <li><b>IMPORTANT! Create a <a href="https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/creating-a-pull-request">Pull Request</a> back to Upstream GitHub Repository</b>
                    <br/>
                    This is how you get your points, so don’t forget it!</li>
                </ul>
            </div>
            <h3>Project Administrator Notes</h3>
            <div>
                <ol>
                    <li>Ignore certain <b>Bug Types</b> using an <code>ignoreRules</code> entry in a <code>.lift.toml</code> file located in the project root directory. See: <a href="https://help.sonatype.com/lift/configuration-reference">Lift Configuration Reference</a>
                        <br/>Note: Ignoring a Bug Type will ignore ALL instances of that bug type.
                    </li>
                </ol>
            </div>
        </>)
    }

    return (
        doRender()
    )

}

export default Instructions;
