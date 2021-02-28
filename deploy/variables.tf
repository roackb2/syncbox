// variables that ingress environment variables
variable "AWS_DEFAULT_REGION" {}
variable "AWS_ACCESS_KEY_ID" {}
variable "AWS_SECRET_ACCESS_KEY" {}
variable "DB_MASTER_USERNAME" {}
variable "DB_MASTER_PWD" {}
variable "SB_SERVER_IMAGE_VERSION" {}
variable "SB_DB_PORT" {}
variable "SB_DB_USER" {}
variable "SB_DB_PWD" {}
variable "SB_DB_DATABASE" {}

// variables for configuration flexibility
variable "project_name" {
    default = "sb-server"
}
variable "key_name" {
    default = "sb-server"
}
variable "server_instance_type" {
    default = "t2.medium"
}
variable "autoscale_gropu_max_size" {
    default = 5
}
variable "autoscale_group_min_size" {
    default = 2
}
variable "autoscale_group_desire_count" {
    default = 2
}
variable "db_instance_class" {
    default = "db.t2.micro"
}
variable "db_instance_storage_size" {
    default = 10
}
variable "ecs_service_desired_task_count" {
    default = 1
}
