terraform {
  required_version = "~>0.12"
}

provider "aws" {
  version = "2.42.0"
}

resource "random_string" "name" {
  length = 16

  upper   = false
  number  = false
  special = false
}

###
### IAM
###

resource "aws_iam_role" "main" {
  name               = random_string.name.result
  assume_role_policy = data.aws_iam_policy_document.main.json
}

data "aws_iam_policy_document" "main" {
  statement {
    actions = ["sts:AssumeRole", ]
    effect  = "Allow"

    principals {
      type        = "Service"
      identifiers = ["autoscaling.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy_attachment" "main" {
  role       = aws_iam_role.main.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AutoScalingNotificationAccessRole"
}

###
### SQS
###

resource "aws_sqs_queue" "main" {
  name                      = random_string.name.result
  max_message_size          = 2048
  message_retention_seconds = 86400
  receive_wait_time_seconds = 10
}

###
### VPC
###

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_subnet" "all" {
  // Use multiple AZs so the test does not fail, if there a shortage of
  // instances in one.
  for_each = toset(data.aws_availability_zones.available.names)

  vpc_id            = aws_vpc.main.id
  availability_zone = each.key

  // Use a pseudo-random subnet, beased on the AZ name.
  // The sunets will be in 10.0.[0-256].0/24.
  cidr_block = cidrsubnet(aws_vpc.main.cidr_block, 8, parseint(md5(each.key), 16) % 256)
}


###
### Launch Template
###

data "aws_ami" "coreos" {
  owners      = ["595879546273"]
  most_recent = true

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  filter {
    name   = "architecture"
    values = ["x86_64"]
  }
}

resource "aws_launch_template" "main" {
  name_prefix = random_string.name.result
  image_id    = data.aws_ami.coreos.id
}

###
### ASG
###

resource "aws_autoscaling_group" "main" {
  desired_capacity = 4
  max_size         = 4
  min_size         = 2
  min_elb_capacity = 4

  vpc_zone_identifier = [for net in aws_subnet.all : net.id]

  mixed_instances_policy {
    launch_template {
      launch_template_specification {
        launch_template_id = aws_launch_template.main.id
      }

      dynamic "override" {
        for_each = toset([
          "t2.micro",
          "t2.nano",
          "t3.micro",
          "t3.nano",
          "t3a.micro",
          "t3a.nano",
        ])

        content {
          instance_type     = override.value
          weighted_capacity = "1"
        }
      }
    }


    instances_distribution {
      on_demand_percentage_above_base_capacity = 0
    }
  }
}

resource "aws_autoscaling_lifecycle_hook" "node_drainer" {
  name                   = random_string.name.result
  autoscaling_group_name = aws_autoscaling_group.main.name
  default_result         = "CONTINUE"
  heartbeat_timeout      = 3600
  lifecycle_transition   = "autoscaling:EC2_INSTANCE_TERMINATING"

  notification_target_arn = aws_sqs_queue.main.arn
  role_arn                = aws_iam_role.main.arn
}

###
### Output
###

output "autoscaling_group_name" {
  value = aws_autoscaling_group.main.name
}

output "aws_sqs_queue_name" {
  value = aws_sqs_queue.main.name
}
