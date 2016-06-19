
provider "aws" {
    access_key = "${var.AWS_ACCESS_KEY_ID}"
    secret_key = "${var.AWS_SECRET_ACCESS_KEY}"
    region     = "us-east-1"
}

resource "aws_security_group" "sb_server_ports" {
    name = "sb-server-ports"
    description = "allow ports that sb-server uses"

    ingress {
      from_port = 80
      to_port = 80
      protocol = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }

    ingress {
      from_port = 8000
      to_port = 8000
      protocol = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }

    ingress {
      from_port = 3000
      to_port = 3000
      protocol = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }

    ingress {
      from_port = 3306
      to_port = 3306
      protocol = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }

    ingress {
      from_port = 22
      to_port = 22
      protocol = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }

    ingress {
      from_port = 443
      to_port = 443
      protocol = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }

    egress {
        from_port = 80
        to_port = 80
        protocol = "tcp"
        cidr_blocks = ["0.0.0.0/0"]
    }

    egress {
        from_port = 8000
        to_port = 8000
        protocol = "tcp"
        cidr_blocks = ["0.0.0.0/0"]
    }

    egress {
        from_port = 3000
        to_port = 3000
        protocol = "tcp"
        cidr_blocks = ["0.0.0.0/0"]
    }

    egress {
        from_port = 3306
        to_port = 3306
        protocol = "tcp"
        cidr_blocks = ["0.0.0.0/0"]
    }

    egress {
        from_port = 22
        to_port = 22
        protocol = "tcp"
        cidr_blocks = ["0.0.0.0/0"]
    }

    egress {
        from_port = 443
        to_port = 443
        protocol = "tcp"
        cidr_blocks = ["0.0.0.0/0"]
    }

    tags {
        Name = "sb-server"
    }
}

resource "aws_iam_role" "ecsInstanceRole" {
    name = "ecsInstanceRole"
    assume_role_policy = "${file("${path.module}/config/ecsInstanceRoleTrustRelationship.json")}"
}

resource "aws_iam_role_policy" "ecs_instance_policy" {
    name = "ecs_instance_policy"
    role = "${aws_iam_role.ecsInstanceRole.id}"
    policy =  "${file("${path.module}/config/ecsInstanceRole.json")}"
}

resource "aws_iam_policy_attachment" "ecs_instance_attatch" {
    name = "ecs_instance_attatch"
    roles = ["${aws_iam_role.ecsInstanceRole.name}"]
    policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"
}

resource "aws_iam_instance_profile" "ecs_instance_profile" {
    name = "ecs_instance_profile"
    roles = ["${aws_iam_role.ecsInstanceRole.name}"]
}

resource "aws_launch_configuration" "as_conf" {
    name = "sb-server-medium-conf"
    image_id = "ami-2b3b6041"
    instance_type = "t2.medium"
    iam_instance_profile = "${aws_iam_instance_profile.ecs_instance_profile.name}"
    key_name = "sb-server"
    security_groups = ["${aws_security_group.sb_server_ports.id}"]
    user_data = <<EOF
    #!/bin/bash
    sudo yum install -y aws-cli;
    sudo aws s3 cp s3://sb-server-ecs-instance-data/ecs.config /etc/ecs/ecs.config;
EOF
}

resource "aws_elb" "elb" {
    name = "sb-server-elb"
    availability_zones = ["us-east-1a", "us-east-1b", "us-east-1c", "us-east-1e"]
    security_groups = ["${aws_security_group.sb_server_ports.id}"]

    listener {
        instance_port = 8000
        instance_protocol = "tcp"
        lb_port = 8000
        lb_protocol = "tcp"
    }

    listener {
        instance_port = 3000
        instance_protocol = "tcp"
        lb_port = 3000
        lb_protocol = "tcp"
    }

    health_check {
        healthy_threshold = 10
        unhealthy_threshold = 4
        timeout = 10
        target = "TCP:8000"
        interval = 300
    }

    cross_zone_load_balancing = true
    idle_timeout = 60
    connection_draining = true
    connection_draining_timeout = 400

    tags {
        Name = "sb-server-elb"
    }
}

output "elb_addr" {
    value = "${aws_elb.elb.dns_name}"
}

resource "aws_autoscaling_group" "asg" {
    name = "sb-server-medium-asg"
    availability_zones = ["us-east-1a"]
    max_size = 5
    min_size = 2
    health_check_grace_period = 300
    health_check_type = "EC2"
    desired_capacity = 2
    force_delete = true
    load_balancers = ["${aws_elb.elb.name}"]
    launch_configuration = "${aws_launch_configuration.as_conf.name}"

    lifecycle {
        create_before_destroy = true
    }
}

resource "aws_s3_bucket" "sb_server_ecs_instance_data" {
    bucket = "sb-server-ecs-instance-data"
    acl = "private"
}

resource "aws_s3_bucket_object" "ecs_config" {
    bucket = "${aws_s3_bucket.sb_server_ecs_instance_data.id}"
    key = "ecs.config"
    source = "${path.module}/config/ecs.config"
    etag = "${md5(file("${path.module}/config/ecs.config"))}"
}

resource "aws_db_instance" "sb_server_db" {
    allocated_storage    = 10
    engine               = "mysql"
    engine_version       = "5.6.27"
    instance_class       = "db.t2.micro"
    name                 = "syncbox"
    username             = "${var.DB_MASTER_USERNAME}"
    password             = "${var.DB_MASTER_PWD}"
    db_subnet_group_name = "default"
    parameter_group_name = "default.mysql5.6"
    vpc_security_group_ids = ["${aws_security_group.sb_server_ports.id}"]

    tags {
        Name = "sb-server"
    }

    provisioner "local-exec" {
        command = "mysql -h ${aws_db_instance.sb_server_db.address} -P${var.SB_DB_PORT} -u${var.DB_MASTER_USERNAME} -p${var.DB_MASTER_PWD} -e  \"GRANT ALL ON ${SB_DB_DATABASE}.* TO '${SB_DB_USER}'@'%' IDENTIFIED BY '${SB_DB_PWD}'\""
    }
}

output "db_host" {
    value = "${aws_db_instance.sb_server_db.address}"
}

resource "aws_ecs_cluster" "sb_server_cluster" {
    name = "sb-server"
}

resource "aws_ecr_repository" "registry" {
    name = "sb-server"

    provisioner "local-exec" {
        command = "`aws ecr get-login --region ${var.region}`"
    }

    provisioner "local-exec" {
        command = "docker tag sb-server:latest ${replace("${aws_ecr_repository.registry.repository_url}", "https://", "")}:${var.SB_SERVER_IMAGE_VERSION}"
    }

    provisioner "local-exec" {
        command = "docker push ${replace("${aws_ecr_repository.registry.repository_url}", "https://", "")}:${var.SB_SERVER_IMAGE_VERSION}"
    }
}

output "registry" {
    value = "${aws_ecr_repository.registry.repository_url}"
}

resource "aws_iam_role" "ecsServiceRole" {
    name = "ecsServiceRole"
    assume_role_policy = "${file("${path.module}/config/ecsServiceRoleTrustRelationship.json")}"
}

resource "aws_iam_role_policy" "sb_server_policy" {
    name = "sb_server_policy"
    role = "${aws_iam_role.ecsServiceRole.id}"
    policy =  "${file("${path.module}/config/ecsServiceRole.json")}"
}


resource "aws_iam_user" "sb_server_user" {
    name = "sb-server"
}

resource "aws_iam_access_key" "sb_server_key" {
    user = "${aws_iam_user.sb_server_user.name}"
}

resource "aws_iam_user_policy" "sb_server_user_policy" {
    name = "test"
    user = "${aws_iam_user.sb_server_user.name}"
    policy = "${file("${path.module}/config/sb-server-user-policy.json")}"
}

resource "template_file" "sb_server_task_template" {
    template = "${file("${path.module}/config/sb-server-task-def.tpl.json")}"

    vars {
        aws_access_key_id = "${aws_iam_access_key.sb_server_key.id}"
        aws_secret_access_key = "${aws_iam_access_key.sb_server_key.secret}"
        sb_server_host = "${aws_elb.elb.dns_name}"
        sb_server_image = "${replace("${aws_ecr_repository.registry.repository_url}", "https://", "")}:${var.SB_SERVER_IMAGE_VERSION}"
        sb_db_host = "${aws_db_instance.sb_server_db.address}"
        sb_db_port = "${var.SB_DB_PORT}"
        sb_db_user = "${var.SB_DB_USER}"
        sb_db_database = "syncbox"
        sb_db_pwd = "${var.SB_DB_PWD}"
        sb_docker_registry = "${aws_ecr_repository.registry.repository_url}"
    }
}

resource "aws_ecs_task_definition" "sb_server_task" {
    family = "sb-server"
    container_definitions = "${template_file.sb_server_task_template.rendered}"
}

resource "aws_ecs_service" "sb_server_service" {
    name = "sb-server"
    cluster = "${aws_ecs_cluster.sb_server_cluster.id}"
    task_definition = "${aws_ecs_task_definition.sb_server_task.arn}"
    desired_count = 1
    iam_role = "${aws_iam_role.ecsServiceRole.arn}"
    depends_on = ["aws_iam_role_policy.sb_server_policy"]

    load_balancer {
        elb_name = "${aws_elb.elb.name}"
        container_name = "sb-server"
        container_port = 8000
    }
}
