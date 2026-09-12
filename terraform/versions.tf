terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 6.0" }
  }

  backend "s3" {
    bucket       = "oficina-tfstate-706215605178"
    key          = "lambda-auth/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}

provider "aws" {
  region = var.regiao

  default_tags {
    tags = {
      Projeto   = "oficina"
      Fase      = "3"
      Ambiente  = var.ambiente
      ManagedBy = "terraform"
      Repo      = "oficina-lambda-auth"
    }
  }
}
