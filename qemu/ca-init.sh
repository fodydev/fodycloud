#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Create (once) the root certificate of the local test ACME CA.
#
# The root is NAME-CONSTRAINED to fodycloud.internal: even though you trust it
# on your PC, it can only ever vouch for *.fodycloud.internal; browsers and
# curl reject anything else it (or a stolen copy of its key) would sign.
#
# Files (gitignored, survive destroy-vm.sh, so you trust the root only once):
#   qemu/.work/ca/root.crt   public root certificate -> trust this
#   qemu/.work/ca/root.key   private key -> shipped only to the test VM's step-ca
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CA_DIR="${CA_DIR:-$HERE/.work/ca}"
DOMAIN="${DOMAIN:-fodycloud.internal}"

if [[ -s "$CA_DIR/root.crt" && -s "$CA_DIR/root.key" ]]; then
  echo ">> Test CA already exists: $CA_DIR/root.crt"
  exit 0
fi
install -d -m 0700 "$CA_DIR"
umask 077
openssl ecparam -name prime256v1 -genkey -noout -out "$CA_DIR/root.key"
openssl req -x509 -new -key "$CA_DIR/root.key" -sha256 -days 3650 \
  -subj "/O=FodyCloud test/CN=FodyCloud Test Root CA ($DOMAIN only)" \
  -addext "basicConstraints=critical,CA:TRUE,pathlen:1" \
  -addext "keyUsage=critical,keyCertSign,cRLSign" \
  -addext "subjectKeyIdentifier=hash" \
  -addext "nameConstraints=critical,permitted;DNS:$DOMAIN,permitted;DNS:.$DOMAIN" \
  -out "$CA_DIR/root.crt"
chmod 0644 "$CA_DIR/root.crt"
echo ">> Created test CA $CA_DIR/root.crt (valid only for $DOMAIN and *.$DOMAIN)"
openssl x509 -in "$CA_DIR/root.crt" -noout -subject -enddate
