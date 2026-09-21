#!/usr/bin/env bash
set -euo pipefail

# HTTPS public et serveur RustDesk Community (hbbs/hbbr). L'API écoute sur
# 127.0.0.1:8443 derrière Nginx ; ce port interne ne doit pas être public.
# Les ports WebSocket 21118/21119 restent fermés tant qu'aucun client Web
# RustDesk n'est utilisé.
# Attention : Docker publie ses ports via iptables et peut contourner UFW.
# Le Compose doit donc rester limité à 21115-21117 et les règles OCI/NSG
# doivent appliquer la même liste d'autorisation côté cloud.

# Ne jamais activer automatiquement un second gestionnaire de pare-feu sur un
# serveur qui utilise déjà des règles iptables/nftables personnalisées.
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
    ufw allow 22/tcp
    ufw allow 80/tcp
    ufw allow 443/tcp
    ufw allow 21115/tcp
    ufw allow 21116/tcp
    ufw allow 21116/udp
    ufw allow 21117/tcp

    for rule in 8443/tcp 1080/tcp 21118/tcp 21119/tcp \
        31115/tcp 31115/udp 31116/tcp 31116/udp 31117/tcp \
        31118/tcp 31119/tcp; do
        ufw --force delete allow "$rule" >/dev/null 2>&1 || true
    done

    ufw --force enable
    ufw status verbose
elif command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld; then
    firewall-cmd --permanent --add-service=ssh
    firewall-cmd --permanent --add-service=http
    firewall-cmd --permanent --add-service=https
    firewall-cmd --permanent --add-port=21115/tcp
    firewall-cmd --permanent --add-port=21116/tcp
    firewall-cmd --permanent --add-port=21116/udp
    firewall-cmd --permanent --add-port=21117/tcp

    for rule in 8443/tcp 1080/tcp 21118/tcp 21119/tcp \
        31115/tcp 31115/udp 31116/tcp 31116/udp 31117/tcp \
        31118/tcp 31119/tcp; do
        firewall-cmd --permanent --remove-port="$rule" >/dev/null 2>&1 || true
    done

    firewall-cmd --reload
    firewall-cmd --list-all
else
    echo "Aucun gestionnaire pris en charge n'est actif ; aucune règle n'a été modifiée." >&2
    echo "Si le serveur utilise iptables/nftables, auditez et adaptez ses règles existantes sans activer UFW." >&2
    exit 1
fi
