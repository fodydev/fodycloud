# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later

output "dashboard_url" {
  description = "URL of the FodyCloud dashboard."
  value       = "https://dashboard.${var.base_domain}"
}

output "site_urls" {
  description = "The platform's addresses, one role each."
  value = {
    home      = "https://${var.base_domain}"
    dashboard = "https://dashboard.${var.base_domain}"
    admin     = "https://admin.${var.base_domain}"
    docs      = "https://${var.base_domain}/docs"
    api       = "https://api.${var.base_domain}/v1"
    mcp       = "https://mcp.${var.base_domain}/mcp"
  }
}

output "apps_domain" {
  description = "Domain under which student apps get free subdomains (<app>.apps.<base_domain>)."
  value       = "apps.${var.base_domain}"
}

output "dns_records" {
  description = "DNS records to create at your DNS provider."
  value = [
    { type = "A", name = var.base_domain, value = local.public_ip },
    { type = "A", name = "dashboard.${var.base_domain}", value = local.public_ip },
    { type = "A", name = "admin.${var.base_domain}", value = local.public_ip },
    { type = "A", name = "api.${var.base_domain}", value = local.public_ip },
    { type = "A", name = "mcp.${var.base_domain}", value = local.public_ip },
    { type = "A", name = "*.apps.${var.base_domain}", value = local.public_ip },
  ]
}

output "tls_mode" {
  description = "How certificates are obtained."
  value = (local.dns01_enabled ? "Let's Encrypt DNS-01 (${var.acme_dns_provider}), wildcard *.apps.${var.base_domain}" :
  local.acme_enabled ? "Let's Encrypt HTTP-01 per hostname" : "self-signed (ACME disabled)")
}

output "ssh_command" {
  description = "Command to log in to the server."
  value       = "ssh -p ${var.ssh_port} ${var.ssh_user}@${var.host}"
}

output "postgres_password" {
  description = "Password of the `fodycloud` PostgreSQL user (internal network only)."
  value       = random_password.postgres.result
  sensitive   = true
}

output "valkey_password" {
  description = "Password of the Valkey (Redis-compatible) instance (internal network only)."
  value       = random_password.valkey.result
  sensitive   = true
}

output "mcp_url" {
  description = "MCP endpoint for AI agents (Streamable HTTP, Bearer API token)."
  value       = "https://mcp.${var.base_domain}/mcp"
}

output "control_plane_version" {
  description = "Content hash of the control-plane source deployed."
  value       = local.control_plane_version
}

output "admin_password" {
  description = "Initial password of admin_email (change it after first login)."
  value       = var.admin_email != "" ? random_password.admin[0].result : null
  sensitive   = true
}

output "restic_password" {
  description = "Encryption password of the off-site restic repository. Store it somewhere safe: backups are useless without it."
  value       = local.restic_password
  sensitive   = true
}

output "backups" {
  description = "Where backups go."
  value       = local.backup_enabled ? "local + restic s3:${local.backup_s3_url}/${local.backup_bucket}" : "local only (/var/backups/fodycloud); set backup_s3_* for off-site copies"
}
