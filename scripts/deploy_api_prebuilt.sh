#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID} -ne 0 ]]; then
    echo "Ce script doit être exécuté avec sudo." >&2
    exit 1
fi

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
bin_dir="${1:-${repo_root}/prebuilt/api}"

required=(api relaisdesk-backup relaisdesk-emailcheck relaisdesk-dbcheck)
for name in "${required[@]}"; do
    if [[ ! -f "${bin_dir}/${name}" ]]; then
        echo "Binaire manquant : ${bin_dir}/${name}" >&2
        exit 1
    fi
done

if [[ ! -f "${bin_dir}/SHA256SUMS.txt" ]]; then
    echo "Manifeste SHA-256 manquant : ${bin_dir}/SHA256SUMS.txt" >&2
    exit 1
fi

# Couverture : sha256sum --check ne contrôle que les fichiers listés par le
# manifeste. Exiger que chaque binaire installé y figure, sinon un manifeste
# tronqué ou partiel ferait installer des binaires non authentifiés.
for name in "${required[@]}"; do
    if ! tr -d '\r' < "${bin_dir}/SHA256SUMS.txt" | grep -q -E "^[0-9a-fA-F]{64}[[:space:]][ *]?${name}$"; then
        echo "Binaire non couvert par SHA256SUMS.txt : ${name}" >&2
        exit 1
    fi
done

(
    cd "${bin_dir}"
    sha256sum --check SHA256SUMS.txt
)

bash "${repo_root}/scripts/setup_server.sh"

install_binary() {
    local source_path="$1"
    local target_path="$2"
    install -D -m 0755 "${source_path}" "${target_path}.new"
    mv -f "${target_path}.new" "${target_path}"
}

install_binary "${bin_dir}/api" /opt/relaisdesk/api/api
install_binary "${bin_dir}/relaisdesk-backup" /opt/relaisdesk/api/relaisdesk-backup
install_binary "${bin_dir}/relaisdesk-emailcheck" /opt/relaisdesk/api/relaisdesk-emailcheck
install_binary "${bin_dir}/relaisdesk-dbcheck" /opt/relaisdesk/api/relaisdesk-dbcheck

install -D -m 0644 "${repo_root}/api/relaisdesk-api.service" /etc/systemd/system/relaisdesk-api.service
install -D -m 0644 "${repo_root}/api/relaisdesk-backup.service" /etc/systemd/system/relaisdesk-backup.service
install -D -m 0644 "${repo_root}/api/relaisdesk-backup.timer" /etc/systemd/system/relaisdesk-backup.timer
install -D -m 0644 "${repo_root}/scripts/relaisdesk.logrotate" /etc/logrotate.d/relaisdesk

cp -n "${repo_root}/api/.env.example" /etc/relaisdesk/api.env 2>/dev/null || true
cp -n "${repo_root}/scripts/backup.env.example" /etc/relaisdesk/backup.env 2>/dev/null || true
chown root:relaisdesk /etc/relaisdesk/api.env /etc/relaisdesk/backup.env
chmod 0640 /etc/relaisdesk/api.env /etc/relaisdesk/backup.env

if [[ ! -f /etc/relaisdesk/backup.key ]]; then
    /opt/relaisdesk/api/relaisdesk-backup keygen -out /etc/relaisdesk/backup.key
fi
chown root:relaisdesk /etc/relaisdesk/backup.key
chmod 0640 /etc/relaisdesk/backup.key

systemctl daemon-reload
systemctl enable relaisdesk-backup.timer

echo "Binaires API installés sans redémarrer relaisdesk-api."
echo "Le minuteur de sauvegarde est installé mais doit être démarré après la migration de la base."
