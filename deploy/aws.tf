provider "aws" {
  access_key = "${var.AWS_ACCESS_KEY_ID}"
  secret_key = "${var.AWS_SECRET_ACCESS_KEY}"
  region     = "us-east-1"
}

resource "aws_instance" "example" {
  ami           = "ami-2b3b6041"
  instance_type = "t2.medium"
}
