# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later

# Settings derived from the variables, shared by the files shipped to the server.
locals {
  acme_enabled  = var.acme_email != ""
  dns01_enabled = local.acme_enabled && var.acme_dns_provider != ""

  # Resolver used for <app>.apps.<base_domain> and the dashboard.
  # Custom student domains always use the "http" resolver (HTTP-01).
  default_resolver = local.dns01_enabled ? "dns" : (local.acme_enabled ? "http" : "")

  traefik_apps_ip = cidrhost(var.apps_subnet, 2)
  apps_db_ip      = cidrhost(var.apps_subnet, 3) # apps-postgres; apps get the upper half
  apps_ip_range   = cidrsubnet(var.apps_subnet, 1, 1)

  remote_home = var.ssh_user == "root" ? "/root" : "/home/${var.ssh_user}"
  sudo        = var.ssh_user == "root" ? "" : "sudo "
  public_ip   = var.public_ip != "" ? var.public_ip : var.host
  sshd_port   = coalesce(var.server_ssh_port, var.ssh_port)

  restic_password = var.restic_password != "" ? var.restic_password : random_password.restic[0].result
  # Test fixtures: Mailpit catches email, SeaweedFS stands in for S3.
  smtp_host      = var.test_fixtures ? "mailpit" : var.smtp_host
  smtp_port      = var.test_fixtures ? 1025 : var.smtp_port
  backup_s3_url  = var.test_fixtures ? "http://seaweedfs:8333" : var.backup_s3_endpoint
  backup_bucket  = var.test_fixtures ? "fodycloud-backups/test" : var.backup_s3_bucket
  backup_enabled = local.backup_s3_url != "" && local.backup_bucket != ""

  # ACME CA: Let's Encrypt (prod or staging), a private CA, or the local test
  # CA (step-ca in the VM, root from qemu/ca-init.sh) when test_ca_dir is set.
  test_ca = var.test_fixtures && var.test_ca_dir != ""
  acme_ca_server = (
    local.test_ca ? "https://ca.${var.base_domain}:9000/acme/acme/directory" :
    var.acme_ca_server != "" ? var.acme_ca_server :
    var.acme_staging ? "https://acme-staging-v02.api.letsencrypt.org/directory" :
    "" # Let's Encrypt production (Traefik's default)
  )
  acme_ca_pem = local.test_ca ? file("${var.test_ca_dir}/root.crt") : var.acme_ca_root_pem
  test_ca_files = local.test_ca ? {
    "/opt/fodycloud/test-ca/root_ca.crt" = { mode = "0644", content = file("${var.test_ca_dir}/root.crt") }
    # Test-only key, readable by step-ca's unprivileged user in the container.
    "/opt/fodycloud/test-ca/root_ca_key" = { mode = "0644", content = file("${var.test_ca_dir}/root.key") }
  } : {}
}
