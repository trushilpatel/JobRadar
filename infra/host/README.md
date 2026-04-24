# Host bootstrap + backups

One-shot host setup and nightly backup scripts for the single Oracle Cloud VM
running JobRadar. Both scripts are env-driven and idempotent.

## Prerequisites

### Oracle Cloud Security List

Before running `bootstrap.sh`, open these ingress rules on the VM's VCN subnet:

| Protocol | Port | Source      | Purpose                    |
| -------- | ---- | ----------- | -------------------------- |
| TCP      | 22   | Your CIDR   | SSH (admin only)           |
| TCP      | 80   | 0.0.0.0/0   | HTTP (ACME challenges)     |
| TCP      | 443  | 0.0.0.0/0   | HTTPS                      |

Leave all other ports closed. The `firewalld` step in `bootstrap.sh` enforces
the same at the host level.

### DNS records

Create these A records pointing at the VM's public IPv4:

| Name             | Type | Value           |
| ---------------- | ---- | --------------- |
| `app.<domain>`   | A    | `<vm-public-ip>` |
| `grafana.<domain>` | A  | `<vm-public-ip>` |

Let's Encrypt will issue certs via HTTP-01 challenge against these names.

## First-time run

```
# 1) As root on the VM:
export ADMIN_USER=opc
export ADMIN_IP_ALLOWLIST="203.0.113.5/32 198.51.100.0/24"
sudo -E bash infra/host/bootstrap.sh

# 2) Copy the JobRadar .env to /opt/jobradar/.env (chmod 600 is applied by bootstrap).

# 3) Start the stack:
cd /opt/jobradar && docker compose up -d
```

The script installs firewalld, CrowdSec + firewall bouncer, dnf-automatic,
chrony, and hardens SSH and docker daemon. It allowlists every CIDR in
`ADMIN_IP_ALLOWLIST` in CrowdSec **before** the bouncer is enabled so the owner
cannot lock themselves out.

## Env var reference — bootstrap.sh

| Var                  | Default      | Required | Purpose                                                     |
| -------------------- | ------------ | -------- | ----------------------------------------------------------- |
| `ADMIN_USER`         | -            | yes      | Only user permitted to SSH in.                              |
| `ADMIN_IP_ALLOWLIST` | -            | yes      | Space-separated CIDRs allowlisted in CrowdSec.              |
| `SSH_PORT`           | `22`         | no       | SSH listen port (update your Security List if changed).     |
| `OCI_METADATA_BLOCK` | `true`       | no       | Blocks containers from reaching 169.254.169.254.            |
| `CROWDSEC_BAN_DURATION` | `4h`      | no       | Default ban duration referenced by `profiles.yaml`.         |
| `DNF_AUTOMATIC`      | `true`       | no       | Enable the `dnf-automatic-install.timer`.                   |
| `ENV_FILE_PATH`      | `/opt/jobradar/.env` | no | Path chmod 600'd at the end of the run.                  |

## Break-glass recovery

If a CrowdSec ban blocks your SSH (e.g., after a flaky VPN flap):

1. From another IP (tether, alternate VPN exit), SSH in.
2. `sudo cscli decisions delete --all` -- clears every active ban.
3. Add your current CIDR to the allowlist: `sudo cscli decisions add --type allow --duration 8760h --ip <your-cidr>`.
4. Re-run `bootstrap.sh` with an updated `ADMIN_IP_ALLOWLIST` to persist the change.

If SSH itself is wedged, use the Oracle Cloud Console's **Serial Console**
(Compute -> Instance -> Console Connection) to log in and run the above.

## Backups

### Env var reference — backup.sh

| Var                    | Default                  | Required | Purpose                                          |
| ---------------------- | ------------------------ | -------- | ------------------------------------------------ |
| `BACKUP_REPO`          | -                        | yes      | Restic repo URL, e.g. `s3:https://...`.          |
| `BACKUP_PASSWORD_FILE` | -                        | yes      | File (chmod 600) containing the repo password.   |
| `BACKUP_PATHS`         | -                        | yes      | Space-separated absolute paths to back up.       |
| `BACKUP_RETENTION`     | `30d`                    | no       | `restic forget --keep-within` window.            |
| `BACKUP_LOG_DIR`       | `/var/log/jobradar-backup` | no     | Where nightly run logs are written.              |

### Cron

Nightly at 03:00 UTC (edit via `sudo crontab -e`):

```
0 3 * * * BACKUP_REPO=s3:https://s3.example.com/bucket \
          BACKUP_PASSWORD_FILE=/etc/jobradar/restic.pw \
          BACKUP_PATHS="/var/lib/docker/volumes/jobradar_grafanadata /var/lib/docker/volumes/jobradar_prometheusdata /var/lib/docker/volumes/jobradar_caddy_data /opt/jobradar/.env" \
          /opt/jobradar/infra/host/backup.sh >> /var/log/jobradar-backup/cron.log 2>&1
```

Test restore: `restic restore latest --target /tmp/restore-test` on a staging box.
