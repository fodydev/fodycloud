# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later

# Everything shipped to the server: platform files (configuration, scripts,
# systemd units) and the control-plane source, which the server builds itself.
# To add a file: an entry in local.platform_files (see CLAUDE.md).
locals {
  # The platform's whole source, shipped and built on the server (no registry
  # needed). The image also packs it into the archive served at
  # <domain>/source (AGPL-3.0 section 13), so this is an allow-list: never
  # add state files, terraform.tfvars, .terraform/ or qemu/.work/ (secrets).
  repo_root = abspath("${path.module}/..")
  source_list = sort(concat(
    [
      "go.mod", "go.sum", "Dockerfile", "LICENSE", "NOTICE", "REUSE.toml", ".gitignore", ".shellcheckrc",
      "README.md", "CONTRIBUTING.md", "TRADEMARKS.md", "CLAUDE.md",
      "SECURITY.md", "CODE_OF_CONDUCT.md", "GOVERNANCE.md",
      "terraform/README.md", "terraform/terraform.tfvars.example", "terraform/.terraform.lock.hcl",
      "qemu/README.md", "qemu/test.tfvars", "qemu/test-usernet.tfvars",
    ],
    tolist(fileset(local.repo_root, "cmd/**")),
    tolist(fileset(local.repo_root, "internal/**")),
    tolist(fileset(local.repo_root, "docs/**")),
    tolist(fileset(local.repo_root, "LICENSES/**")),
    tolist(fileset(local.repo_root, "scripts/**")),
    tolist(fileset(local.repo_root, ".github/**")),
    tolist(fileset(local.repo_root, "terraform/*.tf")),
    tolist(fileset(local.repo_root, "terraform/files/**")),
    tolist(fileset(local.repo_root, "terraform/templates/**")),
    tolist(fileset(local.repo_root, "qemu/*.sh")),
    tolist(fileset(local.repo_root, "qemu/e2e/**")),
  ))
  source_files = {
    for f in local.source_list : "/opt/fodycloud/src/${f}" => { mode = "0644", content = file("${local.repo_root}/${f}") }
  }
  control_plane_version = substr(sha256(join("", [for f in local.source_list : filesha256("${local.repo_root}/${f}")])), 0, 12)

  files = merge(local.platform_files, local.source_files, local.test_ca_files)

  # Every platform file shipped to the VPS: destination path => { content, mode }.
  platform_files = {
    "/opt/fodycloud/bootstrap.sh" = {
      mode    = "0755"
      content = file("${path.module}/files/bootstrap.sh")
    }
    "/opt/fodycloud/config/host.env" = {
      mode = "0644"
      content = templatefile("${path.module}/templates/host.env.tftpl", {
        data_disk_device      = var.data_disk_device
        swap_size_gb          = var.swap_size_gb
        timezone              = var.timezone
        backup_retention_days = var.backup_retention_days
        restic_image          = var.images.restic
        restic_network        = var.test_fixtures ? "fodycloud-s3" : ""
        apps_subnet           = var.apps_subnet
        apps_ip_range         = local.apps_ip_range
        alert_disk_percent    = var.alert_disk_percent
        alert_memory_percent  = var.alert_memory_percent
        alert_cert_days       = var.alert_cert_days
      })
    }
    "/opt/fodycloud/config/plans.json" = {
      mode = "0644"
      content = jsonencode({
        plans                       = var.plans
        apps_domain                 = "apps.${var.base_domain}"
        apps_network                = "fodycloud-apps"
        default_cert_resolver       = local.default_resolver
        custom_domain_cert_resolver = local.acme_enabled ? "http" : ""
      })
    }
    "/opt/fodycloud/config/apps-pg_hba.conf" = {
      mode = "0644"
      content = templatefile("${path.module}/templates/apps-pg_hba.conf.tftpl", {
        apps_subnet   = var.apps_subnet
        appsdb_subnet = var.appsdb_subnet
      })
    }
    "/opt/fodycloud/compose.yaml" = {
      mode    = "0644"
      content = file("${path.module}/files/compose.yaml")
    }
    "/opt/fodycloud/.env" = {
      mode = "0600"
      content = templatefile("${path.module}/templates/stack.env.tftpl", {
        base_domain            = var.base_domain
        tls_resolver           = local.default_resolver
        traefik_apps_ip        = local.traefik_apps_ip
        apps_db_ip             = local.apps_db_ip
        appsdb_subnet          = var.appsdb_subnet
        apps_db_memory         = var.apps_db_memory
        apps_postgres_password = random_password.apps_postgres.result
        images                 = var.images
        postgres_password      = random_password.postgres.result
        valkey_password        = random_password.valkey.result
        control_plane_version  = local.control_plane_version
        public_ip              = local.public_ip
        custom_domain_resolver = local.acme_enabled ? "http" : ""
        disk_quota             = var.data_disk_device != "" ? "true" : "false"
        build_concurrency      = var.build_concurrency
        builder_cpus           = var.builder_cpus
        builder_memory         = var.builder_memory
        admin_email            = var.admin_email
        smtp_host              = local.smtp_host
        smtp_port              = local.smtp_port
        smtp_username          = var.test_fixtures ? "" : var.smtp_username
        smtp_password          = var.test_fixtures ? "" : var.smtp_password
        mail_from              = var.mail_from
        abuse_cpu_minutes      = var.abuse_cpu_minutes
        signup_limit_per_hour  = var.signup_limit_per_hour
        abandon_grace_days     = var.abandon_grace_days
        abandon_check_interval = var.abandon_check_interval
        operator_name          = var.operator_name
        operator_country       = var.operator_country
        operator_email         = var.operator_contact_email
        operator_data_location = var.operator_data_location
        source_url             = var.source_url
        acme_ca_certs          = local.acme_ca_pem != "" ? "/etc/traefik/acme-ca.pem" : ""
        compose_file           = var.test_fixtures ? "compose.yaml:compose.test.yaml" : "compose.yaml"
      })
    }
    "/opt/fodycloud/config/admin.env" = {
      mode    = "0600"
      content = var.admin_email != "" ? "ADMIN_EMAIL=${var.admin_email}\nADMIN_PASSWORD=${random_password.admin[0].result}\n" : ""
    }
    "/opt/fodycloud/traefik/traefik.yml" = {
      mode = "0644"
      content = templatefile("${path.module}/templates/traefik.yml.tftpl", {
        acme_enabled      = local.acme_enabled
        dns01_enabled     = local.dns01_enabled
        acme_email        = var.acme_email
        acme_ca_server    = local.acme_ca_server
        acme_cert_hours   = local.test_ca ? 24 : 0 # step-ca issues 24-hour certificates
        acme_dns_provider = var.acme_dns_provider
        base_domain       = var.base_domain
      })
    }
    "/opt/fodycloud/traefik/dynamic/wake.yml" = {
      mode = "0644"
      content = templatefile("${path.module}/templates/traefik-wake.yml.tftpl", {
        apps_host_regex = "^[a-z0-9-]+\\.apps\\.${replace(var.base_domain, ".", "\\.")}$"
        tls_resolver    = local.default_resolver
      })
    }
    "/opt/fodycloud/compose.test.yaml" = {
      mode    = "0644"
      content = var.test_fixtures ? templatefile("${path.module}/templates/compose.test.yaml.tftpl", { test_ca = local.test_ca, base_domain = var.base_domain }) : "# test fixtures disabled\n"
    }
    "/opt/fodycloud/traefik/acme-ca.pem" = {
      mode    = "0644"
      content = local.acme_ca_pem
    }
    "/opt/fodycloud/config/backup.env" = {
      mode = "0600"
      content = local.backup_enabled ? join("", [
        "RESTIC_REPOSITORY=s3:${local.backup_s3_url}/${local.backup_bucket}\n",
        "RESTIC_PASSWORD=${local.restic_password}\n",
        "AWS_ACCESS_KEY_ID=${var.test_fixtures ? "test" : var.backup_s3_access_key}\n",
        "AWS_SECRET_ACCESS_KEY=${var.test_fixtures ? "test" : var.backup_s3_secret_key}\n",
      ]) : ""
    }
    "/opt/fodycloud/restore.sh" = {
      mode    = "0755"
      content = file("${path.module}/files/restore.sh")
    }
    "/opt/fodycloud/traefik/dynamic/tls.yml" = {
      mode    = "0644"
      content = file("${path.module}/files/traefik-tls.yml")
    }
    "/opt/fodycloud/traefik/dns.env" = {
      mode    = "0600"
      content = join("", [for k, v in var.acme_dns_env : "${k}=${v}\n"])
    }
    "/opt/fodycloud/backup.sh" = {
      mode    = "0755"
      content = file("${path.module}/files/backup.sh")
    }
    "/opt/fodycloud/health.sh" = {
      mode    = "0755"
      content = file("${path.module}/files/health.sh")
    }
    "/etc/systemd/system/fodycloud-health.service" = {
      mode    = "0644"
      content = file("${path.module}/files/fodycloud-health.service")
    }
    "/etc/systemd/system/fodycloud-health.timer" = {
      mode    = "0644"
      content = file("${path.module}/files/fodycloud-health.timer")
    }
    "/etc/fodycloud/firewall.nft" = {
      mode = "0644"
      content = templatefile("${path.module}/templates/firewall.nft.tftpl", {
        ssh_port        = local.sshd_port
        ssh_cidrs_v4    = [for c in var.admin_ssh_cidrs : c if !strcontains(c, ":")]
        ssh_cidrs_v6    = [for c in var.admin_ssh_cidrs : c if strcontains(c, ":")]
        apps_db_ip      = local.apps_db_ip
        traefik_apps_ip = local.traefik_apps_ip
      })
    }
    "/etc/docker/daemon.json" = {
      mode    = "0644"
      content = file("${path.module}/files/daemon.json")
    }
    "/etc/ssh/sshd_config.d/10-fodycloud.conf" = {
      mode    = "0644"
      content = file("${path.module}/files/sshd-fodycloud.conf")
    }
    "/etc/fail2ban/jail.d/fodycloud.local" = {
      mode    = "0644"
      content = templatefile("${path.module}/templates/fail2ban.local.tftpl", { ssh_port = local.sshd_port })
    }
    "/etc/systemd/resolved.conf.d/90-fodycloud.conf" = {
      mode    = "0644"
      content = "# Managed by OpenTofu (FodyCloud): no LLMNR/mDNS listeners on public interfaces.\n[Resolve]\nLLMNR=no\nMulticastDNS=no\n"
    }
    "/etc/sysctl.d/90-fodycloud.conf" = {
      mode    = "0644"
      content = file("${path.module}/files/sysctl.conf")
    }
    "/etc/apt/apt.conf.d/20auto-upgrades" = {
      mode    = "0644"
      content = "APT::Periodic::Update-Package-Lists \"1\";\nAPT::Periodic::Unattended-Upgrade \"1\";\n"
    }
    "/etc/systemd/system/fodycloud-firewall.service" = {
      mode    = "0644"
      content = file("${path.module}/files/fodycloud-firewall.service")
    }
    "/etc/systemd/system/docker.service.d/10-fodycloud.conf" = {
      mode    = "0644"
      content = file("${path.module}/files/docker-override.conf")
    }
    "/etc/systemd/system/fodycloud-backup.service" = {
      mode    = "0644"
      content = file("${path.module}/files/fodycloud-backup.service")
    }
    "/etc/systemd/system/fodycloud-prune.service" = {
      mode    = "0644"
      content = file("${path.module}/files/fodycloud-prune.service")
    }
    "/etc/systemd/system/fodycloud-prune.timer" = {
      mode    = "0644"
      content = file("${path.module}/files/fodycloud-prune.timer")
    }
    "/etc/systemd/system/fodycloud-backup.timer" = {
      mode    = "0644"
      content = file("${path.module}/files/fodycloud-backup.timer")
    }
  }

  # Self-extracting bundle: writes every file into a private staging dir,
  # then hands over to bootstrap.sh, which installs them and converges the host.
  bundle = join("\n", concat(
    [
      "#!/usr/bin/env bash",
      "set -euo pipefail",
      "umask 077",
      "STAGE=\"$(mktemp -d /root/.fodycloud-stage.XXXXXX)\"",
      "trap 'rm -rf \"$STAGE\"' EXIT",
      "put() { mkdir -p \"$STAGE$(dirname \"$1\")\"; base64 -d > \"$STAGE$1\"; chmod \"$2\" \"$STAGE$1\"; }",
    ],
    [for dest, f in local.files : "put '${dest}' ${f.mode} <<'__FODYCLOUD_B64__'\n${base64encode(f.content)}\n__FODYCLOUD_B64__"],
    ["bash \"$STAGE/opt/fodycloud/bootstrap.sh\" \"$STAGE\"", ""],
  ))
}
