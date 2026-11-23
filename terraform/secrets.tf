# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later

# Generated secrets. They are kept in the OpenTofu state: use an encrypted
# remote backend in production.

resource "random_password" "postgres" {
  length  = 32
  special = false
}

# Superuser of the students' PostgreSQL server (apps-postgres).
resource "random_password" "apps_postgres" {
  length  = 32
  special = false
}

resource "random_password" "valkey" {
  length  = 32
  special = false
}

# Encryption password of the off-site restic repository (when none is given).
resource "random_password" "restic" {
  count   = var.restic_password == "" ? 1 : 0
  length  = 40
  special = false
}

# Initial admin account (only when admin_email is set).
resource "random_password" "admin" {
  count   = var.admin_email != "" ? 1 : 0
  length  = 24
  special = false
}
