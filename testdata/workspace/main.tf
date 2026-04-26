terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.0"
    }
  }

  backend "local" {
    path = "terraform.tfstate"
  }
}

variable "instance_count" {
  type    = number
  default = 1
}

resource "null_resource" "example" {
  count = var.instance_count

  triggers = {
    id = count.index
  }
}
