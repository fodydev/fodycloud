# Fallback for machines without the virtual network: QEMU user-mode networking
# (NET=user qemu/run-vm.sh), reached through port forwards on 127.0.0.1.
host                 = "127.0.0.1"
public_ip            = "203.0.113.10" # only used in the dns_records output
ssh_port             = 2222           # QEMU host forward
server_ssh_port      = 22             # sshd inside the VM
ssh_user             = "root"
ssh_private_key_path = "../qemu/.work/id_ed25519"
base_domain          = "fodycloud.internal"
acme_email           = "" # no ACME with slirp (the host's wildcard DNS points at the bridge VM): self-signed cert
data_disk_device     = "/dev/vdb"
swap_size_gb         = 2
admin_email          = "admin@fodycloud.internal"

# Test fixtures: Mailpit (email) and SeaweedFS (S3 for restic backups) run in the VM.
test_fixtures = true
# Flag CPU abuse after 2 minutes instead of 30 so the e2e test stays short.
abuse_cpu_minutes = 2
# Most test traffic comes from one client address: allow repeated runs within an hour.
signup_limit_per_hour = 200

# Same plans as production, plus "sleepy" (idle sleep after 1 minute), "tiny"
# (storage limits small enough to fill) and
# "unused" (short unused-app delays) for their tests; "free" never sleeps or
# stops here, so other tests are not disturbed.
plans = {
  free     = { cpus = 0.25, memory = "256m", disk = "1G", pids = 128, max_apps = 2, max_custom_domains = 0, sleep_after_minutes = 0, db_size = "100M", db_connections = 5, cache_memory = "32m" }
  student  = { cpus = 1, memory = "1g", disk = "5G", pids = 512, max_apps = 5, max_custom_domains = 5, sleep_after_minutes = 0, volume_size = "1G", db_size = "1G", db_connections = 20, cache_memory = "128m" }
  research = { cpus = 2, memory = "4g", disk = "20G", pids = 1024, max_apps = 10, max_custom_domains = 10, sleep_after_minutes = 0, volume_size = "5G", db_size = "5G", db_connections = 50, cache_memory = "512m" }
  sleepy   = { cpus = 0.25, memory = "256m", disk = "1G", pids = 128, max_apps = 2, max_custom_domains = 0, sleep_after_minutes = 1 }
  # "tiny": storage limits small enough to fill in a test.
  tiny = { cpus = 0.25, memory = "256m", disk = "1G", pids = 128, max_apps = 2, max_custom_domains = 0, sleep_after_minutes = 0, volume_size = "20M", db_size = "20M", db_connections = 5, cache_memory = "16m" }
  # Unused-app rule in minutes instead of weeks: warned after ~3 min without
  # visits or deploys, stopped ~2 min later (abandon_grace_days below).
  unused = { cpus = 0.25, memory = "256m", disk = "1G", pids = 128, max_apps = 2, max_custom_domains = 0, sleep_after_minutes = 0, abandon_after_days = 0.0021 }
}
abandon_grace_days     = 0.0014
abandon_check_interval = "1m"

# Example operator for the legal policies.
operator_name          = "FodyCloud Test Operator"
operator_country       = "Cameroon"
operator_contact_email = "privacy@fodycloud.internal"
operator_data_location = "a test VM"

# Sign-in with a forge: a dummy GitHub app (the e2e suite checks the button and
# the redirect to github.com, never signs in). The full flow uses the local
# Forgejo as "Codeberg", enabled by qemu/e2e/25-forge-sign-in.sh.
oauth_github = { client_id = "e2e-dummy-client-id", client_secret = "e2e-dummy-client-secret" }
