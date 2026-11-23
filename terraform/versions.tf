# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }

  # State contains generated secrets (database, Valkey, admin and restic passwords).
  # For production, use an encrypted remote backend, e.g.:
  #
  # backend "s3" {
  #   bucket = "fodycloud-tfstate"
  #   key    = "vps/terraform.tfstate"
  #   region = "eu-central-1"
  #   encrypt = true
  # }
}
