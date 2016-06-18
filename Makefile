
base_dockerfile = build/base.Dockerfile
server_dockerfile = build/server.Dockerfile
server_program_name = sb-server
client_program_name = sb-client
aws_default_region = us-east-1
simple_base_image_name=go-base
base_image_name = $(docker_registry)/$(simple_base_image_name)
server_image_name = $(docker_registry)/$(server_program_name)
server_image_with_version = $(server_image_name):$(git_branch_name)-$(git_sha)
server_container_port = 8000
ecr_get_login = $(shell aws ecr get-login --region $(aws_default_region))
docker_registry = $(shell echo $$SB_DOCKER_REGISTRY)
aws_access_key_id = $(shell echo $$AWS_ACCESS_KEY_ID)
aws_secret_access_key = $(shell echo $$AWS_SECRET_ACCESS_KEY)
sb_server_host = $(shell echo $$SB_SERVER_HOST)
sb_db_user = $(shell echo $$SB_DB_USER)
sb_db_pwd = $(shell echo $$SB_DB_PWD)
sb_db_host = $(shell echo $$SB_DB_HOST)
sb_db_port = $(shell echo $$SB_DB_PORT)
sb_db_database = $(shell echo $$SB_DB_DATABASE)
cur_dir = $(shell pwd)
git_branch_name = $(shell echo `branch_name=$$(git symbolic-ref -q HEAD) && \
	branch_name=$${branch_name\#\#refs/heads/} && \
	branch_name=$${branch_name:-unamed_branch} && \
	echo $$branch_name`)
git_sha = $(shell echo `git rev-parse --short HEAD`)
terraform_state_path = deploy/terraform.tfstate
terraform_plan_path = deploy/plan

# shortcut to merge dev branch to master branch
git-merge-dev:
	git add -A
	git commit
	git checkout master
	git merge dev
	git push
	git checkout dev

# show line of code, if user has cloc installed
show-loc:
	cloc . --exclude-dir=vendor,.idea,Godeps,test-target,test-target2,test-target-backup

# login to docker registry on AWS ECS with AWS command line library
aws-docker-login:
	$(ecr_get_login)

# build the base Golang image
build-base: $(base_dockerfile)
	docker build -t $(base_image_name) - < $(base_dockerfile)
	docker tag $(base_image_name) $(simple_base_image_name)

# build the server image
build-server: $(server_dockerfile)
	docker build -f $(server_dockerfile) -t $(server_image_name):latest .
	docker tag $(server_image_name):latest $(server_image_with_version)

# push the server image to AWS ECS registry
push-server:
	docker push $(server_image_name):latest
	docker push $(server_image_with_version)

# run the server in local Docker container
run-server-local:
	docker run -it --rm \
	--name $(server_program_name) \
	-p $(server_container_port):$(server_container_port) \
	-e "AWS_ACCESS_KEY_ID=$(aws_access_key_id)" \
	-e "AWS_SECRET_ACCESS_KEY=$(aws_secret_access_key)" \
	-e "SB_SERVER_HOST=$(sb_server_host)" \
	-e "SB_DB_USER=$(sb_db_user)" \
	-e "SB_DB_PWD=$(sb_db_pwd)" \
	-e "SB_DB_HOST=$(sb_db_host)" \
	-e "SB_DB_PORT=$(sb_db_port)" \
	-e "SB_DB_DATABASE=$(sb_db_database)" \
	$(server_image_name):latest \
	$(server_program_name)

# build the server image and run the server in local container
build-and-run-server: build-server run-server-local

# build the server and excute the Golang installed command of server program
build-and-run-server-ondisk:
	go build .
	go install ./$(server_program_name)
	$(server_program_name)

# run Go build and install for the client application
build-client:
	go build
	go install ./$(client_program_name)

# build the client binaries for windows 386
build-client-for-windows-386:
	GOOS=windows
	GOARCH=386
	go build -o $(client_program_name).exe

# build the client binaries for windows amd64
build-client-for-windows-amd64:
	GOOS=windows
	GOARCH=amd64
	go build -o $(client_program_name).exe

# run the client installed command
run-client:
	$(client_program_name)

# run the second client and watches another directory
run-second-client:
	$(client_program_name) --root_dir=$(cur_dir)/test-target2

# build and install client command and run the client command
build-and-run-client: build-client run-client

# build and install client command and run the client command with watching another folder
build-and-run-second-client: build-client run-second-client

build: build-base build-server build-client

deploy-infra:
	terraform plan -state=${terraform_state_path} -out=${terraform_plan_path} deploy
	terraform apply -state=${terraform_state_path} ${terraform_plan_path}

teardown-infra:
	terraform plan -state=${terraform_state_path} -out=${terraform_plan_path} -destroy deploy
	terraform destroy -state=${terraform_state_path} deploy

show-infra-status:
	terraform show ${terraform_state_path}
