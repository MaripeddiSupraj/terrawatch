terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.0"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.0"
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

# Used by integration tests to create genuine out-of-band drift without
# requiring AWS/GCP/Azure credentials.
resource "local_file" "managed" {
  filename = "${path.module}/managed.txt"
  content  = "managed by terraform\n"
}
