# Syncbox

Term Project for National Taiwan University 2016 course Cloud Computing：Technology and Applications.

Project Goal:
* build file synchronization service
* reliable
* scalable
* highly available
* fault tolerant

Domain Knowledge:
* socket programming
* cloud infrastructure planning
* system architecture design & implementation
* distributed programming

## What is this?

Syncbox is a Dropbox-like service, that enables synchronization between devices for certain folder.
User runs a process by issuing `sb-client` with flags to specify a folder to watch, and the user authentication information,
then the service would synchronize this folder across the user's devices.


## Building Project

## Prerequisite

1. Golang:
This project is written in Golang, users who want to build the project should have their Golang environment correctly setup, including the `$GOROOT` and the `$GOPATH` environment variables.

2. Docker:
Also, the server is intended to be run in Docker containers, so developer should have their local Docker environment ready.

3. AWS CLI:
Makefile commands rely on AWS Command Line Tools to communicate with AWS.

4. AWS S3:
The server defaults to store files in S3, user should have their S3 service ready for development.

5. MySQL:
The server defaults to store relations in MySQL database, you could user AWS RDS for this.

6. Environment Variables:
The server and client takes some environment variables to identify server host, storage, database ip, etc.

7. Terraform:
This project use Terraform to deploy infra on AWS.

## Steps
* `go get github.com/roackb2/syncbox`
* `cd "$GOPATH"/src/github.com/roackb2/syncbox`
* exports environment variables for Makefile commands and Terraform commands, like following:
```shell
export AWS_ACCESS_KEY_ID="[your aws access key]"
export AWS_SECRET_ACCESS_KEY="[your aws secret key]"
export SB_DB_USER="[db user name]"
export SB_DB_PWD="[db user password]"
export SB_DB_PORT="3306"
export SB_DB_DATABASE="[database]"
export TF_VAR_DB_MASTER_USERNAME="[master user name of RDS]"
export TF_VAR_DB_MASTER_PWD="[master user pwd of RDS]"
export TF_VAR_AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID
export TF_VAR_AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY
export TF_VAR_SB_DB_USER=$SB_DB_USER
export TF_VAR_SB_DB_PWD=$SB_DB_PWD
export TF_VAR_SB_DB_PORT=$SB_DB_PORT
export TF_VAR_SB_DB_DATABASE=$SB_DB_DATABASE
```
content inside brackets (including the brackets) should be substituted with real values, depending on your development environment.

* `make build-base`, this builds a base image with Golang image and network utilities installed, to speed up further buildings.
* `make build-and-run-server`, this would run the server in local Docker container
* `mkdir test-target`, the client application default to  watch content of this folder and synchronize it.
* open a new terminal session, issue `make build-and-run-client-with-local-server`, this would build the client application and run the Go installed command of the client application.

## Deployment of Server Application

* Quick Deployment

    issue `make deploy-infra`, this would deploy the infrastructure on AWS. This is achieved by using [Terraform](https://www.terraform.io/) to automate the deployment process.

* Cloud Native Deployment

    The server is intended to be run in cloud native way, which means it should run in Docker containers. The development process was established by running on AWS ECS, you could also use Container Service of Google Cloud Platform or any bare-metal machines with container orchestration mechanism like Kubernetes to serve as the backend. In short, any cloud native backend structure would be suitable to run the server.

* Local Development

    Local development could run the server inside containers on user's local machine.

* Makefile Commands

    The Makefile contains targets for easy building for local development and remote deployment, see comments in Makefile to know more about available commands

## Identified Problems & Solution

1. Packet Segmentation

    Packet may be segmented and have to call multiple read on the socket to get the full packet, this happens if the packet size exceeds the buffer size.

    Solution: This is solved by adding a protocol that enforce fixed packet size, and loop through socket reading until a full packet is read.
2. Packet Splicing

    Multiple packets might be concatenated and will be read together via single read operation on the socket.

    Solution: This is also solved by adding protocol to limit packet size and only reads fixed length message.
3. Packet Interleaving

    Packets from different messages might interleaves if the messages come from different sending source and sends simultaneously.

    Solution: This is solved by adding message ID to packets, dispatch packets to their queuing message and assemble them back to a message.

4. Unstable Connection

    Long living socket connection might be cut off due to server side connection policy.

    Solution: This is solved by try to dial again if the client side attempt to send a message and find that connection is broken, the server side is not able to dial to the client side, the failure of message sending relies on application level error handling.

## Limitation

* Modification While Syncing

    Currently if user modify files on one device, it has to wait for another device to be totally synchronized for further modification, otherwise modification may be overwritten by the newer version.

*   Large File Synchronization

    Also, transporting granularity is file, large files would now fail due to operation timeout (how large the file could be transported depends on network bandwidth). Future work might try to improve granularity to chunks to support large file synchronization.

## History

The repository on Github is a clean copy of the original repository on Gitlab(not a clone), to prevent leak of credentials.
