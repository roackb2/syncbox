include envs/prod.Makefile
-include local/Makefile
base_dockerfile = build/base.Dockerfile
server_dockerfile = build/server.Dockerfile
server_program_name = sb-server
client_program_name = sb-client
simple_base_image_name = go-base
git_branch_name = $(shell echo `branch_name=$$(git symbolic-ref -q HEAD) && \
	branch_name=$${branch_name\#\#refs/heads/} && \
	branch_name=$${branch_name:-unamed_branch} && \
	echo $$branch_name`)
git_sha = $(shell echo `git rev-parse --short HEAD`)
server_image_name = $$AWS_REGISTRY_HOSTNAME/$(server_program_name)
server_image_version = $(git_branch_name)-$(git_sha)
server_image_with_version = $(server_image_name):$(server_image_version)
server_container_port = 8000
sb_db_host = $(shell terraform output -state=${terraform_state_path} db_host)
cur_dir = $(shell pwd)
aws_ecr_login_cmd = aws ecr get-login-password | docker login --username AWS --password-stdin $$AWS_REGISTRY_HOSTNAME;
connect_db_command := mysql -h $$SB_DB_HOST --port=$$SB_DB_PORT --user=$$SB_DB_USER --password=$$SB_DB_PWD --database=$$SB_DB_DATABASE
terraform_state_path = terraform.tfstate
terraform_plan_path = plan

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

# build the base Golang image
build-base: $(base_dockerfile)
	$(setup_prod_env) \
	docker build -t $(simple_base_image_name) - < $(base_dockerfile)

# build the server image
build-server: $(server_dockerfile)
	$(setup_prod_env) \
	docker build -f $(server_dockerfile) -t $(server_program_name):latest .

# push the server image to AWS ECS registry
push-server:
	$(setup_prod_env) \
	$(aws_ecr_login_cmd) \
	docker tag $(server_program_name):latest $(server_image_with_version); \
	docker push $(server_image_name):latest; \
	docker push $(server_image_with_version);

# run the server in local Docker container
run-server-local:
	$(setup_prod_env) \
	docker run -it --rm \
	--name $(server_program_name) \
	-p $(server_container_port):$(server_container_port) \
	-e "AWS_DEFAULT_REGION=$$AWS_DEFAULT_REGION" \
	-e "AWS_ACCESS_KEY_ID=$$AWS_ACCESS_KEY_ID" \
	-e "AWS_SECRET_ACCESS_KEY=$$AWS_SECRET_ACCESS_KEY" \
	-e "SB_DB_USER=$$SB_DB_USER" \
	-e "SB_DB_PWD=$$SB_DB_PWD" \
	-e "SB_DB_HOST=$$SB_DB_HOST" \
	-e "SB_DB_PORT=$$SB_DB_PORT" \
	-e "SB_DB_DATABASE=$$SB_DB_DATABASE" \
	-e "SB_STORAGE_BUCHET"=$$SB_STORAGE_BUCHET \
	$(server_program_name):latest \
	$(server_program_name)

# build the server image and run the server in local container
build-and-run-server: build-server run-server-local

# build the server and excute the Golang installed command of server program
build-and-run-server-ondisk:
	go build .
	go install ./$(server_program_name);
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

# run the client installed command and connects to remote server
run-client:
	$(setup_prod_env) \
	$(client_program_name)

# run the client installed command and connects to local server
run-client-with-local-server:
	SB_SERVER_HOST=localhost \
	$(client_program_name)

# run the second client and watches another directory
run-second-client:
	$(setup_prod_env) \
	$(client_program_name) --root_dir=$(cur_dir)/test-target2

# connect to production database
connnect-prod-db:
	$(setup_prod_env) \
	$(connect_db_command)

# build and install client command and run the client command
build-and-run-client: build-client run-client

# build and install client command and run the client command that connects to local server
build-and-run-client-with-local-server: build-client run-client-with-local-server


# build and install client command and run the client command with watching another folder
build-and-run-second-client: build-client run-second-client

build: build-base build-server build-client

# deploy infrastructure on AWS
deploy-infra:
	$(setup_prod_env) \
	cd deploy; \
	terraform init; \
	terraform get; \
	terraform apply -backup=-;

# refresh infra status from cloud
refresh-infra:
	$(setup_prod_env) \
	cd deploy; \
	terraform init; \
	terraform get; \
	terraform refresh -backup=-;	

# teardown infrastructure that deployed on AWS
teardown-infra:
	$(setup_prod_env) \
	cd deploy; \
	terraform destroy

# show infrastructure status
show-infra-status:
	$(setup_prod_env) \
	terraform show ${terraform_state_path}
