# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later

resource "terraform_data" "provision" {
  # Re-provision whenever any shipped file (including secrets) or the target
  # server changes. To re-provision a rebuilt server at the same address, run
  # `tofu apply -replace=terraform_data.provision`.
  triggers_replace = {
    bundle = sha256(local.bundle)
    host   = var.host
    port   = var.ssh_port
  }

  connection {
    type        = "ssh"
    host        = var.host
    port        = var.ssh_port
    user        = var.ssh_user
    private_key = file(pathexpand(var.ssh_private_key_path))
    host_key    = var.ssh_host_key
    timeout     = "5m"
  }

  # Uploaded into the SSH user's home directory (not world-readable /tmp)
  # because the bundle contains secrets; removed right after the run.
  provisioner "file" {
    content     = local.bundle
    destination = "${local.remote_home}/.fodycloud-bundle.sh"
  }

  provisioner "remote-exec" {
    inline = [
      "chmod 600 ${local.remote_home}/.fodycloud-bundle.sh",
      "${local.sudo}bash ${local.remote_home}/.fodycloud-bundle.sh; rc=$?; rm -f ${local.remote_home}/.fodycloud-bundle.sh; exit $rc",
    ]
  }
}
