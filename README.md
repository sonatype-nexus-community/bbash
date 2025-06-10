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
[![sonatype-nexus-community](https://circleci.com/gh/sonatype-nexus-community/bbash.svg?style=shield)](https://circleci.com/gh/sonatype-nexus-community/bbash)
# BBash

Tooling to enable a Bug Bash

## Development

### Setup

To get started with this project you will need:

- Make (on macOS, you could use [brew](https://brew.sh) via `brew install make` should suffice).
- Golang (see: [Download and Install](https://go.dev/doc/install). This project started using Go 1.16.2, but likely anything above 1.14 is fine). Using 1.17.8 now. Nope, now up to 1.24.1.
- Docker (see: [Get Docker](https://docs.docker.com/get-docker/))
- Npm (see: [Node and npm](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm))

  On Ubuntu, you could use these commands to get the latest stable node version setup:

      sudo apt install npm
      sudo npm cache clean -f
      sudo npm install -g n
      sudo n stable

  You may have to restart the terminal after running the above steps to see the latest node version. 
- Yarn: `sudo npm install --global yarn` (see: [Installation](https://classic.yarnpkg.com/en/docs/install))
- Air

    To install air:

    - https://github.com/cosmtrek/air

    You can run:

    - `go install github.com/cosmtrek/air@v1.29.0`

    The `air` binary will be located in your `~/go/bin` folder, which may need to added to your commands and/or path.
    The [AIRCMD](Makefile#L6) setting in the Makefile may need to be adjusted if a different location is used. 

### Running/Developing

Thanks to Air, there is some amount of "live-reload". To run the project, you can run `air -c .air.toml` in the project root. 
Once it is built, you should be able to access the site at http://localhost:7777/.
The app pages live at: http://localhost:7777/index.html

Any code changes to golang files will cause a rebuild and restart, and will be accessible via the browser with a refresh!

#### Local Development Setup
For local development, a good first step is to copy the example `.env.example` file to `.env` and launch a local db
and `air` like so:
```shell
cp .env.example .env
make run-air
```

Note that Datadog polling, which is used to calculate scores, is disabled by default in `.env.example` to decrease noise during local development.
To re-enable [remove this line](.env.example#L18).

#### Server Debugging
For some fun interactive debugging of the golang app with [server.go](./server.go), you could spin up the local docker db image, and manually run
the server in debug mode. See the [Makefile](./Makefile) for the latest and greatest commands to cherry-pick.
```shell
$ docker run --name bug_bash_postgres -p 5432:5432 -e POSTGRES_PASSWORD=bug_bash -e POSTGRES_DB=db -d postgres
b6ac8769bab3b19b3e5818e726272bcee6957863b9a7af4261a0ae29ec5bc68e...
```
Then run [server.go](./server.go) in debug mode in your favorite IDE, and enjoy break points activating when you connect to 
endpoints. Wee!

#### Frontend Development
For frontend work (with a previously manually launched database - see `docker run ...` above), this command is helpful for development:
```shell
make run-air-alone
```

## Architecture

"Two apps in one" - This project contains two apps:
  1. A [golang](https://go.dev) application that provides REST endpoints for the UI and admin tasks, and polls [Lift](https://lift.sonatype.com/getting-started) for scoring events.
  2. A [react](https://reactjs.org) application that provides a UI, and calls the REST endpoints served by the golang app. 

#### Go

  The go application specific files include:

   * [server.go](./server.go)
   * [internal](./internal/)
   * [go.mod](./go.mod)
    
  The go application communicates with the postgres database. The go application also 
  periodically polls the Lift logs for scoring events.

#### React

  The react application files include:

  * [src](./src/)
  * [public](./public/)
  * [package.json](./package.json)
  * [yarn.lock](./yarn.lock)
   

## Deployment

#### App environment configuration

Configuration of `bbash` is handled via a `.env` file in the repo (this is ignored by git by default, so you don't check in secrets):

A `.example.env` has been provided that looks similar to the following:

```
PG_USERNAME=postgres
PG_PASSWORD=bug_bash
PG_PORT=5432
PG_DB_NAME=db
PG_HOST=localhost
SSL_MODE=disable
```

### Deploy Application to AWS

Thankfully, we've made this as simple as possible, we think? It'll get simpler with time, I'm sure :)

You will need:

- `terraform`
- `aws cli`
- `aws-vault`
- `docker`

* Sonatype employees see [here](https://docs.sonatype.com/display/DEVOPSKB/AWS%3A+Getting+Started+with+AWS+at+Sonatype) for access request instructions and two factor authentication setup.

#### Terraform

- `aws-vault exec <your_profile> terraform init`
- `aws-vault exec <your_profile> terraform apply`

This should create all the nice lil AWS resources to manage this application, using ECS and ECR!

#### Docker

To create the docker image:

- `make docker`

#### Deployment

Some pre-requisite/one-time setup steps:

  * setup aws cli configuration to verify working credentials. see: [AWS CLI on mac](https://docs.aws.amazon.com/cli/latest/userguide/install-macos.html)

  * install `aws-vault`
        
        $ brew install --cask aws-vault
        
  * create AWS profile for "<your_profile>" below
       In AWS under Account -> "Security Credentials" -> “Access keys for CLI, SDK, & API access”

  * add aws-vault profile ("<your_profile>" in steps below) for use in pushing images

        $ aws-vault add my-bbash-profile
        
       For sonatype employees: make sure to set up two factor auth [(see link)](https://docs.sonatype.com/display/DEVOPSKB/aws-vault+Introduction)

  * (One-time) initialize terraform

        $ aws-vault exec <your_profile> terraform init

  * View terraform actions to be taken:

        $ aws-vault exec <your_profile> terraform plan


An executable bash script (`docker.sh`?) similar to the following will make pushing images easier:

```bash
#!/bin/bash
aws-vault exec <your_profile> aws ecr get-login-password --region <aws_region> | docker login --username AWS --password-stdin <aws_account_id>.dkr.ecr.<aws_region>.amazonaws.com
docker tag bug-bash:latest <aws_account_id>.dkr.ecr.<aws_region>.amazonaws.com/bug-bash-app:latest
docker push <aws_account_id>.dkr.ecr.<aws_region>.amazonaws.com/bug-bash-app:latest
aws-vault exec <your_profile> -- aws ecs update-service --cluster bug-bash-cluster --service bug-bash-service --force-new-deployment
```

Replace the stuff in the `<>` with your values (and remove the `<>` characters if that isn't immediately apparent), `chmod +x docker.sh`, and `./docker.sh`

After you have done this, you SHOULD have a running service, somewhere in AWS :) - maybe someplace like this? :
[sandbox-dev](https://bug-bash.innovations-sandbox.sonatype.dev) or [sandbox-dev/index.html](https://bug-bash.innovations-sandbox.sonatype.dev/index.html) 

With all the above configured, here's the deployment command in full:

    make && make docker && ./docker.sh
    
Please note that `make docker` will also increment the version number of this build and create a commit for this change.

### Viewing log files in AWS (for newer users)
* For Sonatype employees make sure to Switch Roles to [innovations-sandbox](https://docs.sonatype.com/display/SRE/AWS+Innovation+Sandbox). Under main menu select "Switch Roles". Enter account number (12 digits) and role (ie admin). Please note that if using a Mac you may need to be on Safari browser for this to work.

In AWS console search for "CloudWatch".

From CloudWatch navigate to logs -> log groups -> bug-bash-cloudwatch-lergs.

### Helpful Links:

* [Echo](https://echo.labstack.com) web framework. [repo](https://github.com/labstack/echo)
* [How To Run A Campaign](docs/howto-create-campaign.md)
* Locally runnable [CI docs](.circleci/circleci-readme.md) 

## The Fine Print

It is worth noting that this is **NOT SUPPORTED** by Sonatype, and is a contribution of ours
to the open source community (read: you!)

Remember:

* Use this contribution at the risk tolerance that you have
* Do NOT file Sonatype support tickets related to `bbash` support in regard to this project
* DO file issues here on GitHub, so that the community can pitch in

Phew, that was easier than I thought. Last but not least of all:

Have fun creating and using `bbash`, we are glad to have you here!
