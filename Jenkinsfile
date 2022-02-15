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
  buildImageId: "${sonatypeDockerRegistryId()}/cdi/golang-1.17.1:2",
  deployBranch: 'main',
  prepare: {
    githubStatusUpdate('pending')
  },
  buildAndTest: {
    sh '''
    make all
    go get -u github.com/jstemmer/go-junit-report
    make test | go-junit-report > test-results.xml
    '''
  },
  vulnerabilityScan: {
    withDockerImage(env.DOCKER_IMAGE_ID, {
      withCredentials([usernamePassword(credentialsId: 'jenkins-iq',
        usernameVariable: 'IQ_USERNAME', passwordVariable: 'IQ_PASSWORD')]) {
        sh 'npx auditjs@latest iq -x -a auditjs -s release -u $IQ_USERNAME -p $IQ_PASSWORD -h https://iq.sonatype.dev'
      }
    })
  },
  testResults: [ 'test-results.xml' ],
  onSuccess: {
    githubStatusUpdate('success')
  },
  onFailure: {
    githubStatusUpdate('failure')
//    notifyChat(currentBuild: currentBuild, env: env, room: 'community-oss-fun')
//    sendEmailNotification(currentBuild, env, [], 'community-group@sonatype.com')
  }
)
