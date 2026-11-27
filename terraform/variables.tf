# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later

# ---------------------------------------------------------------------------
# Connection to the existing VPS
# ---------------------------------------------------------------------------

variable "host" {
  description = "Public IP address or hostname of the VPS."
  type        = string
}

variable "public_ip" {
  description = "Public IPv4 of the VPS used for the DNS records output. Defaults to `host`."
  type        = string
  default     = ""
}

variable "ssh_port" {
  description = "SSH port of the VPS."
  type        = number
  default     = 22

  validation {
    condition     = var.ssh_port >= 1 && var.ssh_port <= 65535
    error_message = "ssh_port must be a TCP port (1-65535)."
  }
}

variable "server_ssh_port" {
  description = "Port sshd listens on inside the server (opened in the firewall and watched by fail2ban). Defaults to `ssh_port`; set it when you connect through NAT/port forwarding."
  type        = number
  default     = null

  validation {
    condition     = var.server_ssh_port == null || (var.server_ssh_port >= 1 && var.server_ssh_port <= 65535)
    error_message = "server_ssh_port must be null or a TCP port (1-65535)."
  }
}

variable "ssh_user" {
  description = "SSH user. Must be root or a user with passwordless sudo."
  type        = string
  default     = "root"
}

variable "ssh_private_key_path" {
  description = "Path to the private key used to connect to the VPS."
  type        = string
  default     = "~/.ssh/id_ed25519"
}

variable "ssh_host_key" {
  description = "Optional public host key of the VPS (e.g. `ssh-ed25519 AAAA...`). When set, Terraform verifies the server identity."
  type        = string
  default     = null
}

# ---------------------------------------------------------------------------
# Platform
# ---------------------------------------------------------------------------

variable "base_domain" {
  description = "Root domain of the platform. The home page and the guides (/docs) are served on it, with dashboard., admin., api. and mcp.<base_domain> for the other sites; student apps on <app>.apps.<base_domain>."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$", var.base_domain))
    error_message = "base_domain must be a valid lowercase domain name, e.g. fodycloud.example."
  }
}

variable "acme_email" {
  description = "Email for Let's Encrypt. Empty disables ACME and Traefik serves a self-signed certificate (useful for local tests)."
  type        = string
  default     = ""

  validation {
    condition     = var.acme_email == "" || can(regex("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$", var.acme_email))
    error_message = "acme_email must be empty (self-signed certificates) or an email address."
  }
}

variable "acme_staging" {
  description = "Use the Let's Encrypt staging CA (no rate limits, untrusted certificates)."
  type        = bool
  default     = false
}

variable "acme_ca_server" {
  description = "ACME directory URL of a private CA (e.g. a university's internal step-ca). Empty = Let's Encrypt."
  type        = string
  default     = ""
}

variable "acme_ca_root_pem" {
  description = "PEM root certificate of the private ACME CA, so Traefik can reach it over TLS. Empty for public CAs."
  type        = string
  default     = ""
}

variable "acme_dns_provider" {
  description = "Optional lego DNS provider code (e.g. `cloudflare`, `ovh`, `hetzner`) enabling a DNS-01 resolver and a wildcard certificate for *.apps.<base_domain>. Empty uses HTTP-01 per hostname."
  type        = string
  default     = ""
}

variable "acme_dns_env" {
  description = "Environment variables for the DNS provider credentials, e.g. { CF_DNS_API_TOKEN = \"...\" }."
  type        = map(string)
  default     = {}
  sensitive   = true
}

variable "plans" {
  description = "Resource limits per subscription plan, published to /opt/fodycloud/config/plans.json for the control plane."
  type = map(object({
    cpus                = number
    memory              = string
    disk                = string
    pids                = number
    max_apps            = number
    max_custom_domains  = number # custom domains per app (0 = none)
    sleep_after_minutes = number # idle minutes before the app sleeps (0 = never); it wakes on the next request
    # Days without visits or deploys before an app is warned, then stopped
    # (see abandon_grace_days); 0 = never.
    abandon_after_days = optional(number, 0)
    # Storage and databases ("" = not included): a persistent volume at /data,
    # a managed PostgreSQL database (size and connections) and a Valkey cache.
    volume_size    = optional(string, "")
    db_size        = optional(string, "")
    db_connections = optional(number, 0)
    cache_memory   = optional(string, "")
  }))
  default = {
    free = {
      cpus                = 0.25
      memory              = "256m"
      disk                = "1G"
      pids                = 128
      max_apps            = 2
      max_custom_domains  = 0
      sleep_after_minutes = 30
      abandon_after_days  = 30
      db_size             = "100M"
      db_connections      = 5
      cache_memory        = "32m"
    }
    student = {
      cpus                = 1
      memory              = "1g"
      disk                = "5G"
      pids                = 512
      max_apps            = 5
      max_custom_domains  = 5
      sleep_after_minutes = 0
      volume_size         = "1G"
      db_size             = "1G"
      db_connections      = 20
      cache_memory        = "128m"
    }
    research = {
      cpus                = 2
      memory              = "4g"
      disk                = "20G"
      pids                = 1024
      max_apps            = 10
      max_custom_domains  = 10
      sleep_after_minutes = 0
      volume_size         = "5G"
      db_size             = "5G"
      db_connections      = 50
      cache_memory        = "512m"
    }
  }
}

variable "appsdb_subnet" {
  description = "IPv4 subnet of the internal network between the control plane and the students' PostgreSQL server (apps-postgres). Must not overlap apps_subnet or Docker's address pools."
  type        = string
  default     = "172.29.255.240/28"

  validation {
    condition     = can(cidrhost(var.appsdb_subnet, 2))
    error_message = "appsdb_subnet must be an IPv4 CIDR block, e.g. 172.29.255.240/28."
  }
}

variable "apps_db_memory" {
  description = "Memory limit of the students' PostgreSQL server (apps-postgres), shared by every app's database."
  type        = string
  default     = "1g"
}

variable "apps_subnet" {
  description = "IPv4 subnet of the shared student-app network. Traefik gets the .2 address; apps are allocated from the upper half."
  type        = string
  default     = "172.30.0.0/16"

  validation {
    condition     = can(cidrhost(var.apps_subnet, 2))
    error_message = "apps_subnet must be an IPv4 CIDR block, e.g. 172.30.0.0/16."
  }
}

variable "images" {
  description = "Container images of the platform stack."
  type = object({
    traefik      = string
    socket_proxy = string
    postgres     = string
    valkey       = string
    restic       = string
    probe        = optional(string, "busybox:1.37.0") # runs the apps' HTTP health checks (busybox nc)
  })
  default = {
    traefik      = "traefik:v3.6"
    socket_proxy = "tecnativa/docker-socket-proxy:lts"
    postgres     = "postgres:17-alpine"
    valkey       = "valkey/valkey:8.1-alpine"
    restic       = "restic/restic:0.19.1"
    probe        = "busybox:1.37.0"
  }
}

# ---------------------------------------------------------------------------
# Host
# ---------------------------------------------------------------------------

variable "data_disk_device" {
  description = "Optional block device (e.g. /dev/sdb, /dev/vdb) formatted as XFS with project quotas and mounted on /var/lib/docker. Required for per-app disk quotas. It is formatted only if it carries no filesystem."
  type        = string
  default     = ""

  validation {
    condition     = var.data_disk_device == "" || can(regex("^/dev/[A-Za-z0-9/_-]+$", var.data_disk_device))
    error_message = "data_disk_device must be empty or a /dev/... path."
  }
}

variable "swap_size_gb" {
  description = "Size of the swap file in GB (0 disables)."
  type        = number
  default     = 4

  validation {
    condition     = var.swap_size_gb >= 0
    error_message = "swap_size_gb must be 0 (no swap) or more."
  }
}

variable "timezone" {
  description = "Server timezone."
  type        = string
  default     = "Africa/Douala"
}

variable "admin_ssh_cidrs" {
  description = "Source networks allowed to reach SSH."
  type        = list(string)
  default     = ["0.0.0.0/0", "::/0"]
}

variable "backup_retention_days" {
  description = "Days to keep the daily PostgreSQL dumps in /var/backups/fodycloud."
  type        = number
  default     = 7

  validation {
    condition     = var.backup_retention_days >= 1
    error_message = "backup_retention_days must be at least 1."
  }
}

# ---------------------------------------------------------------------------
# Control plane
# ---------------------------------------------------------------------------

variable "admin_email" {
  description = "Email of the initial admin account (admin panel at admin.<base_domain>; admins are separate from student accounts). Created on first apply with a generated password (`tofu output -raw admin_password`). Empty skips it."
  type        = string
  default     = ""

  validation {
    condition     = var.admin_email == "" || can(regex("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$", var.admin_email))
    error_message = "admin_email must be empty (no initial admin) or an email address."
  }
}

variable "build_concurrency" {
  description = "Number of student builds that may run at the same time."
  type        = number
  default     = 2

  validation {
    condition     = var.build_concurrency >= 1
    error_message = "build_concurrency must be at least 1."
  }
}

variable "builder_cpus" {
  description = "CPU limit of the shared BuildKit builder container."
  type        = number
  default     = 2

  validation {
    condition     = var.builder_cpus > 0
    error_message = "builder_cpus must be more than 0."
  }
}

variable "builder_memory" {
  description = "Memory limit of the shared BuildKit builder container."
  type        = string
  default     = "4g"
}

# ---------------------------------------------------------------------------
# Email (verification, password reset, plan reminders, admin alerts)
# ---------------------------------------------------------------------------

variable "smtp_host" {
  description = "SMTP server. Empty disables email; email verification is then not required."
  type        = string
  default     = ""
}

variable "smtp_port" {
  description = "SMTP port (587 = STARTTLS, 465 = implicit TLS)."
  type        = number
  default     = 587
}

variable "smtp_username" {
  description = "SMTP username (credentials are only sent over TLS)."
  type        = string
  default     = ""
}

variable "smtp_password" {
  description = "SMTP password."
  type        = string
  default     = ""
  sensitive   = true
}

variable "mail_from" {
  description = "Sender address, e.g. \"FodyCloud <no-reply@fodycloud.example>\". Defaults to no-reply@<base_domain>."
  type        = string
  default     = ""
}

# ---------------------------------------------------------------------------
# Off-site backups (restic to S3-compatible storage)
# ---------------------------------------------------------------------------

variable "backup_s3_endpoint" {
  description = "S3 endpoint URL (e.g. https://s3.eu-central-003.backblazeb2.com). Empty keeps backups on the server only."
  type        = string
  default     = ""
}

variable "backup_s3_bucket" {
  description = "Bucket (and optional path) for the restic repository, e.g. fodycloud-backups/prod."
  type        = string
  default     = ""
}

variable "backup_s3_access_key" {
  description = "S3 access key ID."
  type        = string
  default     = ""
  sensitive   = true
}

variable "backup_s3_secret_key" {
  description = "S3 secret access key."
  type        = string
  default     = ""
  sensitive   = true
}

variable "restic_password" {
  description = "Encryption password of the restic repository. Generated when empty (tofu output -raw restic_password): KEEP IT, backups cannot be restored without it."
  type        = string
  default     = ""
  sensitive   = true
}

# ---------------------------------------------------------------------------
# Abuse protection and tests
# ---------------------------------------------------------------------------

variable "abuse_cpu_minutes" {
  description = "Consecutive minutes an app may run at >= 90% of its CPU limit before it is flagged (free-plan apps flagged twice in 24 h are stopped)."
  type        = number
  default     = 30

  validation {
    condition     = var.abuse_cpu_minutes >= 1
    error_message = "abuse_cpu_minutes must be at least 1."
  }
}

variable "test_ca_dir" {
  description = "Local tests only (with test_fixtures): directory holding root.crt/root.key from qemu/ca-init.sh. Runs step-ca in the VM and points Traefik's ACME client at it."
  type        = string
  default     = ""
}

variable "signup_limit_per_hour" {
  description = "Signups and password-reset requests allowed per client IP per hour."
  type        = number
  default     = 5

  validation {
    condition     = var.signup_limit_per_hour >= 1
    error_message = "signup_limit_per_hour must be at least 1."
  }
}

variable "test_fixtures" {
  description = "Local tests only: run Mailpit (SMTP catcher) and SeaweedFS (S3) next to the platform. Never enable in production."
  type        = bool
  default     = false
}

# ---------------------------------------------------------------------------
# Unused apps, platform alerts, operator (legal policies)
# ---------------------------------------------------------------------------
variable "abandon_grace_days" {
  description = "Days between the 'your app will be stopped' email and the stop of an unused app (plans set abandon_after_days)."
  type        = number
  default     = 7

  validation {
    condition     = var.abandon_grace_days > 0
    error_message = "abandon_grace_days must be more than 0."
  }
}

variable "abandon_check_interval" {
  description = "How often the unused-app rule runs (Go duration, e.g. 1h; the test VM uses 1m)."
  type        = string
  default     = "1h"

  validation {
    condition     = can(regex("^[0-9]+(s|m|h)$", var.abandon_check_interval))
    error_message = "abandon_check_interval must be a duration such as 30m or 1h."
  }
}

variable "alert_disk_percent" {
  description = "Email the admins when a disk (/ or /var/lib/docker) is at least this full, in percent."
  type        = number
  default     = 85

  validation {
    condition     = var.alert_disk_percent > 0 && var.alert_disk_percent <= 100
    error_message = "alert_disk_percent must be between 1 and 100."
  }
}

variable "alert_memory_percent" {
  description = "Email the admins when at least this much memory is in use, in percent."
  type        = number
  default     = 90

  validation {
    condition     = var.alert_memory_percent > 0 && var.alert_memory_percent <= 100
    error_message = "alert_memory_percent must be between 1 and 100."
  }
}

variable "alert_cert_days" {
  description = "Email the admins when a TLS certificate expires in fewer days than this (Let's Encrypt renews 30 days ahead)."
  type        = number
  default     = 14

  validation {
    condition     = var.alert_cert_days >= 0
    error_message = "alert_cert_days must be 0 (disabled) or more."
  }
}

variable "operator_name" {
  description = "Legal name of whoever runs this platform, named in the terms of service and privacy policy (e.g. \"FodyDev SARL\")."
  type        = string
  default     = ""
}

variable "operator_country" {
  description = "Country whose laws govern the terms of service (e.g. \"Cameroon\")."
  type        = string
  default     = ""
}

variable "operator_contact_email" {
  description = "Contact address for privacy requests and abuse reports, shown in the policies."
  type        = string
  default     = ""

  validation {
    condition     = var.operator_contact_email == "" || can(regex("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$", var.operator_contact_email))
    error_message = "operator_contact_email must be empty or an email address."
  }
}

variable "operator_data_location" {
  description = "Where the platform's data is stored, shown in the privacy policy (e.g. \"Cameroon (VPS at Example Hosting)\")."
  type        = string
  default     = ""
}

variable "source_url" {
  description = "Public repository of the FodyCloud source this platform runs, linked from <domain>/source with the exact source archive. If you run a modified version, point it to your fork (AGPL-3.0)."
  type        = string
  default     = "https://github.com/fodydev/fodycloud"

  validation {
    condition     = can(regex("^https://[^\\s\"]+$", var.source_url))
    error_message = "source_url must be an https:// URL."
  }
}

# ---------------------------------------------------------------------------
# Sign-in with a forge (optional). Each provider is off until you register an
# OAuth app on it and set its client ID and secret here. Students can always
# log in with email and password; remove a block to turn a provider off.
# Callback URL to register: https://dashboard.<base_domain>/auth/<github|gitlab|codeberg>/callback
# ---------------------------------------------------------------------------

variable "oauth_github" {
  description = "GitHub OAuth app (github.com > Settings > Developer settings > OAuth Apps). null = no \"Continue with GitHub\"."
  type = object({
    client_id     = string
    client_secret = string
  })
  default   = null
  sensitive = true

  validation {
    condition     = var.oauth_github == null || (can(regex("^[^\\s\"']+$", try(var.oauth_github.client_id, ""))) && can(regex("^[^\\s\"']+$", try(var.oauth_github.client_secret, ""))))
    error_message = "oauth_github needs both client_id and client_secret (no spaces or quotes), or null."
  }
}

variable "oauth_gitlab" {
  description = "GitLab OAuth application (scope read_user). url: the instance, default https://gitlab.com; label: the button's name, default \"GitLab\". null = no \"Continue with GitLab\"."
  type = object({
    client_id     = string
    client_secret = string
    url           = optional(string, "")
    label         = optional(string, "")
  })
  default   = null
  sensitive = true

  validation {
    condition = var.oauth_gitlab == null || (
      can(regex("^[^\\s\"']+$", try(var.oauth_gitlab.client_id, ""))) && can(regex("^[^\\s\"']+$", try(var.oauth_gitlab.client_secret, ""))) &&
      can(regex("^(https://[^\\s\"/?#]+(/[^\\s\"?#]*)?)?$", try(var.oauth_gitlab.url, ""))) &&
      can(regex("^[^\"\\n]{0,40}$", try(var.oauth_gitlab.label, "")))
    )
    error_message = "oauth_gitlab needs client_id and client_secret (no spaces or quotes); url must be empty or an https:// URL, label at most 40 characters without quotes."
  }
}

variable "oauth_codeberg" {
  description = "Codeberg (or another Forgejo/Gitea) OAuth2 application. url: the instance, default https://codeberg.org; label: the button's name, default \"Codeberg\". null = no \"Continue with Codeberg\"."
  type = object({
    client_id     = string
    client_secret = string
    url           = optional(string, "")
    label         = optional(string, "")
  })
  default   = null
  sensitive = true

  validation {
    condition = var.oauth_codeberg == null || (
      can(regex("^[^\\s\"']+$", try(var.oauth_codeberg.client_id, ""))) && can(regex("^[^\\s\"']+$", try(var.oauth_codeberg.client_secret, ""))) &&
      can(regex("^(https://[^\\s\"/?#]+(/[^\\s\"?#]*)?)?$", try(var.oauth_codeberg.url, ""))) &&
      can(regex("^[^\"\\n]{0,40}$", try(var.oauth_codeberg.label, "")))
    )
    error_message = "oauth_codeberg needs client_id and client_secret (no spaces or quotes); url must be empty or an https:// URL, label at most 40 characters without quotes."
  }
}
