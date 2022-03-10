<!--

    Copyright (c) 2021-present Sonatype, Inc.

    Licensed under the Apache License, Version 2.0 (the "License");
    you may not use this file except in compliance with the License.
    You may obtain a copy of the License at

        http://www.apache.org/licenses/LICENSE-2.0

    Unless required by applicable law or agreed to in writing, software
    distributed under the License is distributed on an "AS IS" BASIS,
    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    See the License for the specific language governing permissions and
    limitations under the License.

-->

# How To Run A Campaign

Here are some minimal steps to set up a new Bug Bash Campaign.

1. Create a new Campaign record:

       curl -u "theAdminUsername:theAdminPassword" -X PUT http://localhost:7777/admin/campaign/add/myCampaignName -d '{ "startOn": "2021-03-10T12:00:00.000Z", "endOn": "2022-03-15T12:00:00.000Z"}'

2. Add the organization that owns the repository that will having the bug bash:

       curl -u "theAdminUsername:myAdminPassword" -X PUT http://localhost:7777/admin/organization/add -d '{ "scpName": "GitHub", "organization": "my-organization"}'

3. Add at least one participant (e.g. a github user) who will be submitting bug fixes during this campaign:

       curl -u "theAdminUsername:myAdminPassword" -X PUT http://localhost:7777/admin/participant/add -d '{ "scpName": "GitHub", "campaignName": "myCampaignName", "loginName": "mygithubid"}'

To verify things are working, have the GitHub user ("mygithubid" in this example) generate a Pull Request against a 
repository owned by the organization we added ("my-organization" in this example).