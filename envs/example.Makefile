setup_prod_env := export SB_DB_USER="syncbox"; \
  export SB_DB_PWD="syncbox"; \
  export SB_DB_HOST="localhost"; \
  export SB_DB_PORT="3306"; \
  export SB_DB_DATABASE="syncbox"; \
  export SB_STORAGE_BUCHET="syncbox"; \
  export AWS_DEFAULT_REGION="ap-northeast-1"; \
  export AWS_ACCESS_KEY_ID="key_id"; \
  export AWS_SECRET_ACCESS_KEY="secret_key"; \
  export AWS_REGISTRY_HOSTNAME=test.com; \
  export SB_SERVER_HOST=test.com; \
  export TF_VAR_DB_MASTER_USERNAME="username"; \
  export TF_VAR_DB_MASTER_PWD="password"; \
  export TF_VAR_AWS_DEFAULT_REGION=$$AWS_DEFAULT_REGION; \
  export TF_VAR_AWS_ACCESS_KEY_ID=$$AWS_ACCESS_KEY_ID; \
  export TF_VAR_AWS_SECRET_ACCESS_KEY=$$AWS_SECRET_ACCESS_KEY; \
  export TF_VAR_SB_DB_USER=$$SB_DB_USER; \
  export TF_VAR_SB_DB_PWD=$$SB_DB_PWD; \
  export TF_VAR_SB_DB_PORT=$$SB_DB_PORT; \
  export TF_VAR_SB_DB_DATABASE=$$SB_DB_DATABASE; \
  export TF_VAR_SB_SERVER_IMAGE_VERSION=$(server_image_version);