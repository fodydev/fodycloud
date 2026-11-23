# Local test platform (QEMU)

These scripts run a real FodyCloud platform in a Debian 13 VM on your computer, on a virtual
network that behaves like the internet, and test it end to end.

**Requirements:** Go 1.27, OpenTofu, `qemu-system-x86_64`, `qemu-img`, `genisoimage`, `jq`,
`curl`, `python3`, NetworkManager, and KVM (recommended).

## Scripts

| Script | What it does |
|---|---|
| `net-setup.sh` | **Once, as root.** Creates the virtual network, the wildcard DNS and the local CA (below). Idempotent. |
| `net-teardown.sh` | Removes everything `net-setup.sh` created. |
| `ca-init.sh` | Creates the local test CA in `.work/ca/` (called by `net-setup.sh`). |
| `run-vm.sh` | Starts the VM at `203.0.113.10` (downloads the Debian image once). |
| `destroy-vm.sh` | Stops the VM and deletes its disks. |
| `smoke-test.sh` | Infrastructure checks: host hardening, firewall, TLS, isolation, quotas, backups. |
| `e2e-test.sh` | Product checks (about 20 minutes): build and deploy a real repository, stack review, plans, email, abuse, sleep, admin site, API, MCP, dashboard, backups. |
| `testlib.sh` | Shared by both suites: connection settings, VM lock, `check`, `remote`, `section`, `finish`. |
| `e2e/` | The e2e checks, one file per area, sourced in order (`NN-name.sh`), plus `lib.sh` (API/MCP helpers). |
| `test.tfvars`, `test-usernet.tfvars` | OpenTofu settings for the VM (bridge and user-mode networks). |

## The virtual network

```bash
sudo qemu/net-setup.sh        # or from a root shell: qemu/net-setup.sh [--user NAME]
```

It creates a NetworkManager bridge on **203.0.113.0/24** (TEST-NET-3: never routed on the
internet, and not private, so FodyCloud treats it as public):

| Address | Role |
|---|---|
| `203.0.113.1` | your computer, the "internet" gateway (NAT to the real internet) |
| `203.0.113.2`–`.5` | extra client addresses, so tests act as different internet users |
| `203.0.113.10` | the VM, i.e. "the VPS", with real ports 22, 80 and 443 |

It also sets up:

- **Wildcard DNS:** `*.fodycloud.internal → 203.0.113.10`, through NetworkManager's dnsmasq.
  `.internal` is reserved for private use; `.local` would be answered by mDNS first.
- **A local certificate authority**, name-constrained to `fodycloud.internal` and trusted by your
  system (and Chromium with `libnss3-tools`; import `.work/ca/root.crt` into Firefox yourself).
  In the VM, step-ca issues certificates through the same ACME HTTP-01 flow as Let's Encrypt, so
  HTTPS is real.

QEMU runs unprivileged, on a TAP device owned by you.

## Test cycle

```bash
qemu/run-vm.sh
tofu -chdir=terraform apply -var-file=../qemu/test.tfvars
qemu/smoke-test.sh             # must end "N passed, 0 failed"
qemu/e2e-test.sh               # idem, ~20 minutes
tofu -chdir=terraform plan -detailed-exitcode -var-file=../qemu/test.tfvars   # must exit 0
qemu/destroy-vm.sh
```

- **Don't touch the VM while a suite runs.** The suites lock it (`.work/test.lock`), so
  `run-vm.sh` and `destroy-vm.sh` refuse; if the VM is rebooted anyway, the suite stops with
  `ABORT` instead of reporting false failures.
- **Re-created VM, same address:** run
  `tofu -chdir=terraform apply -replace=terraform_data.provision -var-file=../qemu/test.tfvars`.
- **No NetworkManager?** `NET=user qemu/run-vm.sh` with `-var-file=../qemu/test-usernet.tfvars`:
  the VM is reached on `127.0.0.1:2222/8080/8443`, with self-signed certificates.

## Write a check

Every check is one line: `check "what is verified" COMMAND…`. It passes when the command
succeeds; on failure its last output lines are shown. Put it in the `e2e/` file of its area, or
add a new `NN-area.sh` (files run in name order and share variables). Checks that something must
fail need a positive control, so they can't pass just because a tool is missing.

## Browse the test platform

Open `https://fodycloud.internal`. The admin panel is `https://admin.fodycloud.internal`
(`admin@fodycloud.internal`, password from `tofu -chdir=terraform output -raw admin_password`).

Test-only services (`test_fixtures = true`): **Mailpit** catches all email
(`ssh -i qemu/.work/id_ed25519 -N -L 8025:127.0.0.1:8025 root@203.0.113.10`, then
`http://localhost:8025`); **SeaweedFS** stands in for S3.
