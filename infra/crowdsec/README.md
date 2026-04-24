# CrowdSec config

Custom scenarios + acquisition + remediation profile for JobRadar.

## Files

| File | Purpose |
| ---- | ------- |
| `acquis.yaml` | Tells CrowdSec which log files to tail (Caddy + CrowdSec own logs). |
| `profiles.yaml` | Default IP/range ban duration. |
| `scenarios/jobradar-api-401-abuse.yaml` | Ban IPs hitting `/api/*` with 401 repeatedly. |
| `scenarios/jobradar-grafana-login-abuse.yaml` | Ban IPs failing Grafana OAuth repeatedly. |

## Env vars referenced by bootstrap / ops

CrowdSec scenario and profile YAML files do **not** support env interpolation
natively. The values below are the defaults baked into the YAML; to override,
edit the YAML and restart CrowdSec.

| Var                       | Default | Hardcoded in                                   | Purpose                                  |
| ------------------------- | ------- | ---------------------------------------------- | ---------------------------------------- |
| `API_401_THRESHOLD`       | `10`    | `scenarios/jobradar-api-401-abuse.yaml`        | 401s per minute before ban.              |
| `GRAFANA_LOGIN_THRESHOLD` | `5`     | `scenarios/jobradar-grafana-login-abuse.yaml`  | Failed logins per minute before ban.     |
| `CROWDSEC_BAN_DURATION`   | `4h`    | `profiles.yaml`                                | How long a ban lasts.                    |

## Install

On the host, symlink these files into CrowdSec's config tree:

```
sudo ln -sf /opt/jobradar/infra/crowdsec/acquis.yaml /etc/crowdsec/acquis.yaml
sudo ln -sf /opt/jobradar/infra/crowdsec/profiles.yaml /etc/crowdsec/profiles.yaml
sudo install -m 0644 /opt/jobradar/infra/crowdsec/scenarios/*.yaml /etc/crowdsec/scenarios/
sudo systemctl reload crowdsec
```

## Verify

```
sudo cscli scenarios list | grep jobradar
sudo cscli metrics
sudo cscli decisions list       # live decisions
sudo cscli alerts list          # recent alerts
```

## Tuning

Start permissive and tighten -- a 401-heavy deploy hour (e.g., post-token-rotation)
should not auto-ban the owner. If it does, the ADMIN_IP_ALLOWLIST from
`bootstrap.sh` protects against that.
