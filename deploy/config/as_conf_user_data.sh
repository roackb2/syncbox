#!/bin/bash
sudo yum install -y aws-cli;
sudo aws s3 cp s3://sb-server-ecs-instance-data/ecs.config /etc/ecs/ecs.config;
