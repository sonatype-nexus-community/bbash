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
@Library(['private-pipeline-library', 'jenkins-shared']) _

dockerizedBuildPipeline(
  pathToDockerfile: "jenkins.dockerfile",
  deployBranch: 'main',
  buildAndTest: {
    sh '''
    make all
    '''
  },
  vulnerabilityScan: {
    withDockerImage(env.DOCKER_IMAGE_ID, {
      withCredentials([usernamePassword(credentialsId: 'jenkins-saas-service-acct',
        usernameVariable: 'IQ_USERNAME', passwordVariable: 'IQ_PASSWORD')]) {
        sh 'npx auditjs@latest iq -x -a bbash -s release -u $IQ_USERNAME -p $IQ_PASSWORD -h https://sonatype.sonatype.app/platform'
        sh 'go list -json -deps | /tmp/tools/nancy iq --iq-application bbash-services --iq-stage release --iq-username $IQ_USERNAME --iq-token $IQ_PASSWORD --iq-server-url https://sonatype.sonatype.app/platform'
      }
    })
  },
  onFailure: {
    notifyChat(currentBuild: currentBuild, env: env, room: 'community-oss-fun')
    sendEmailNotification(currentBuild, env, [], 'community-group@sonatype.com')
  }
)
