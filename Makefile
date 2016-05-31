
base_dockerfile = build/base.Dockerfile
server_dockerfile = build/server.Dockerfile
server_program_name = sb-server
client_program_name = sb-client
aws_default_region = us-east-1
simple_base_image_name=go-base
base_image_name = $(docker_registry)/$(simple_base_image_name)
server_image_name = $(docker_registry)/$(server_program_name)
version = v2
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

git-merge-dev:
	git add -A
	git commit
	git checkout master
	git merge dev
	git push
	git checkout dev

show-loc:
	cloc . --exclude-dir=vendor,.idea,Godeps,test-target

aws-docker-login:
	$(ecr_get_login)

build-base: $(base_dockerfile)
	docker build -t $(base_image_name) - < $(base_dockerfile)
	docker tag $(base_image_name) $(simple_base_image_name)

build-server: $(server_dockerfile)
	docker build -f $(server_dockerfile) -t $(server_image_name):latest .
	docker tag $(server_image_name):latest $(server_image_name):$(version)

push-server:
	docker push $(server_image_name):latest
	docker push $(server_image_name):$(version)

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

build-and-run-server: build-server run-server-local

build-and-run-server-ondisk:
	go build .
	go install ./$(server_program_name)
	$(server_program_name)

build-client:
	go build
	go install ./$(client_program_name)

build-client-for-windows-386:
	GOOS=windows
	GOARCH=386
	go build -o $(client_program_name).exe

build-client-for-windows-amd64:
	GOOS=windows
	GOARCH=amd64
	go build -o $(client_program_name).exe

run-client:
	$(client_program_name)

run-second-client:
	$(client_program_name) --root_dir=test-target2

build-and-run-client: build-client run-client

build-and-run-second-client: build-client run-second-client



build: build-base build-server build-client
