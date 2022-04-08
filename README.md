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
- Golang (project started using Go 1.16.2, but likely anything above 1.14 is fine). Using 1.17.8 now.
- Air

    To install air:

    - https://github.com/cosmtrek/air

    You can run:

    - `go get -u github.com/cosmtrek/air` in a folder outside this project (so it is not added as a dependency).

    The `air` binary will be located in your `~/go/bin` folder, which may need to added to your commands and/or path.
    The [AIRCMD](Makefile#L6) setting in the Makefile may need to be adjusted if a different location is used. 

### Running/Developing

Thanks to Air, there is some amount of "live-reload". To run the project, you can run `air -c .air.toml` in the project root. 
Once it is built, you should be able to access the site at http://localhost:7777/.
The app pages live at: http://localhost:7777/index.html

Any code changes to golang files will cause a rebuild and restart, and will be accessible via the browser with a refresh!

For local development, a good first step is to copy the example `.env.example` file to `.env` and launch a local db
and `air` like so:
```shell
cp .env.example .env
make run-air
```

For some fun interactive debugging with [server.go](./server.go), you could spin up the local docker db image, and manually run
the server in debug mode. See the [Makefile](./Makefile) for the latest and greatest commands to cherry-pick.
```shell
$ docker run --name bug_bash_postgres -p 5432:5432 -e POSTGRES_PASSWORD=bug_bash -e POSTGRES_DB=db -d postgres
b6ac8769bab3b19b3e5818e726272bcee6957863b9a7af4261a0ae29ec5bc68e...
```
Then run [server.go](./server.go) in debug mode in your favorite IDE, and enjoy break points activating when you connect to 
endpoints. Wee!

For frontend work (with a previously manually launched database), this command is helpful for development:
```shell
make run-air-alone
```

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

  * add aws-vault profile ("<your_profile>" in steps below) for use in pushing images

        $ aws-vault add my-bbash-profile

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

### Helpful Links:

* [Echo](https://echo.labstack.com) web framework. [repo](https://github.com/labstack/echo)
* [How To Run A Campaign](docs/howto-create-campaign.md)

## The Fine Print

It is worth noting that this is **NOT SUPPORTED** by Sonatype, and is a contribution of ours
to the open source community (read: you!)

Remember:

* Use this contribution at the risk tolerance that you have
* Do NOT file Sonatype support tickets related to `bbash` support in regard to this project
* DO file issues here on GitHub, so that the community can pitch in

Phew, that was easier than I thought. Last but not least of all:

Have fun creating and using `bbash`, we are glad to have you here!
