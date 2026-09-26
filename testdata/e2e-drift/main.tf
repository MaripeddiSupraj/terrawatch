terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "~> 2.5"
    }
  }
}

variable "content" {
  type    = string
  default = "desired-v1"
}

resource "local_file" "managed" {
  filename = "${path.module}/managed.txt"
  content  = var.content
}
