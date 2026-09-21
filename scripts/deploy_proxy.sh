#!/usr/bin/env bash
set -euo pipefail

echo "ERREUR: le proxy SOCKS5 a été retiré de l'architecture RelaisDesk." >&2
echo "Déployez hbbs/hbbr et ouvrez 21115/tcp, 21116/tcp+udp et 21117/tcp." >&2
exit 1
