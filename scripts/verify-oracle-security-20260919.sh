#!/usr/bin/env bash
set -euo pipefail
sudo systemctl show relaisdesk-api.service -p MainPID -p ActiveState -p SubState -p NRestarts --no-pager
sudo docker inspect rustdesk-hbbs rustdesk-hbbr | python3 -c '
import json,sys
for c in json.load(sys.stdin):
    e=dict(v.split("=",1) for v in c["Config"]["Env"] if "=" in v)
    assert e.get("RELAISDESK_AUTH_REQUIRED", "").lower() in ("y","1","yes","true")
    assert e.get("RELAISDESK_AUTH_PUBLIC_KEYS")
    assert c["State"]["Running"] and c["RestartCount"] == 0
    assert c["HostConfig"]["ReadonlyRootfs"]
    print(c["Name"], c["Config"]["Image"], "auth=required", "restarts=0", "read_only=true", "source="+e.get("RELAISDESK_SOURCE_URL",""))
'
sudo docker exec rustdesk-hbbs sha256sum /usr/local/bin/hbbs
sudo docker exec rustdesk-hbbr sha256sum /usr/local/bin/hbbr
sudo docker logs --since 3m rustdesk-hbbs 2>&1 | grep -E 'Listening|Start|error|ERROR|panicked|Authorization|RelaisDesk' || true
sudo docker logs --since 3m rustdesk-hbbr 2>&1 | grep -E 'Listening|Start|error|ERROR|panicked|Authorization|RelaisDesk' || true
sudo ss -lntup '( sport = :8443 or sport = :21115 or sport = :21116 or sport = :21117 )'
sudo systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --no-pager
sudo sqlite3 -readonly /data/relaisdesk/licences.db "PRAGMA quick_check; SELECT 'licences',COUNT(*) FROM licences UNION ALL SELECT 'devices',COUNT(*) FROM devices UNION ALL SELECT 'orders',COUNT(*) FROM orders UNION ALL SELECT 'accounts',COUNT(*) FROM customer_accounts;"
curl -fsS --max-time 10 https://api.relaisdesk.fr/api/v1/health
echo
