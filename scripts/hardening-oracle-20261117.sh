#!/usr/bin/env bash
# Durcissement du serveur Oracle (Ubuntu 22.04) - 2026-11-17.
#
# A executer SUR LE SERVEUR, en root, apres validation par l'exploitant,
# UNE sous-commande a la fois. Chaque etape est idempotente.
#
#   sudo bash hardening-oracle-20261117.sh <sous-commande>
#
# Ordre recommande :
#   backup -> nginx -> sshd (tester une 2e session) -> certbot -> iptables
#   -> upgrade -> verify
# Sous-commandes isolees : certbot-delete-duckdns, rollback-nginx,
# rollback-sshd, rollback-iptables, rollback-certbot-cron, reboot-and-check.
#
# Ce script ne redemarre JAMAIS le serveur et ne touche ni au site
# informatiqueadomicile03.fr (PHP/MariaDB), ni a WireGuard, ni aux conteneurs
# hbbs/hbbr (hors mise a jour Docker de l'etape upgrade, voir le rapport).
set -euo pipefail

STAMP_DAY="20261117"
BACKUP_ROOT="/root"
NGINX_CONF="/etc/nginx/nginx.conf"
LEGACY_NAME="api.informatiqueadomicile03.fr"
LEGACY_VHOST="/etc/nginx/sites-available/${LEGACY_NAME}"
MAIN_NAME="api.relaisdesk.fr"
SSHD_DROPIN="/etc/ssh/sshd_config.d/90-relaisdesk-hardening.conf"
IPT_RULES="/etc/iptables/rules.v4"
SNAP_CERTBOT="/snap/bin/certbot"
HEALTH_PATH="/api/v1/health"
DUCKDNS_CERT="relaisdesk.duckdns.org"

umask 077

log()  { printf '\033[1;34m[INFO]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[ OK ]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[WARN]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m[FAIL]\033[0m %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }

require_root() {
    [[ $EUID -eq 0 ]] || die "A executer en root (sudo)."
}

# Sauvegarde la plus ancienne = etat d'origine, avant tout changement.
# Peut etre forcee avec BACKUP_DIR=/root/hardening-...
oldest_backup() {
    if [[ -n "${BACKUP_DIR:-}" ]]; then
        [[ -d "$BACKUP_DIR" ]] || die "BACKUP_DIR introuvable : $BACKUP_DIR"
        printf '%s\n' "$BACKUP_DIR"
        return
    fi
    local d
    d=$(ls -1d "${BACKUP_ROOT}"/hardening-${STAMP_DAY}-* 2>/dev/null | sort | head -n1 || true)
    [[ -n "$d" ]] || return 1
    printf '%s\n' "$d"
}

require_backup() {
    oldest_backup >/dev/null 2>&1 || die "Aucune sauvegarde trouvee. Lancer d'abord : $0 backup"
    log "Sauvegarde de reference : $(oldest_backup)"
}

# --------------------------------------------------------------------------
# backup
# --------------------------------------------------------------------------
cmd_backup() {
    require_root
    local ts dest
    ts=$(date +%Y%m%d-%H%M%S)
    dest="${BACKUP_ROOT}/hardening-${STAMP_DAY}-${ts}"
    mkdir -p "$dest"
    chmod 700 "$dest"

    cp -a /etc/nginx "$dest/etc-nginx"
    cp -a /etc/letsencrypt "$dest/etc-letsencrypt"
    mkdir -p "$dest/etc-ssh"
    cp -a /etc/ssh/sshd_config "$dest/etc-ssh/sshd_config"
    cp -a /etc/ssh/sshd_config.d "$dest/etc-ssh/sshd_config.d"
    if [[ -d /etc/iptables ]]; then
        cp -a /etc/iptables "$dest/etc-iptables"
    fi
    iptables-save  > "$dest/iptables-live.v4"
    ip6tables-save > "$dest/iptables-live.v6" 2>/dev/null || true
    crontab -l -u root > "$dest/root-crontab.txt" 2>/dev/null || echo "# (pas de crontab root)" > "$dest/root-crontab.txt"
    systemctl list-timers --all --no-pager > "$dest/timers.txt" 2>/dev/null || true
    dpkg -l > "$dest/dpkg-list.txt" 2>/dev/null || true
    uname -r > "$dest/kernel.txt"

    chmod -R go-rwx "$dest"
    ok "Sauvegarde creee : $dest (permissions root uniquement)"
    du -sh "$dest" | sed 's/^/      taille : /'
}

# --------------------------------------------------------------------------
# nginx
# --------------------------------------------------------------------------
curl_health() { # $1=domaine -> code HTTP via 127.0.0.1 (independant du DNS)
    curl -sS -o /dev/null -m 10 -w '%{http_code}' \
        --resolve "$1:443:127.0.0.1" "https://$1${HEALTH_PATH}" 2>/dev/null || echo "000"
}

restore_nginx_files() {
    local b
    b=$(oldest_backup) || die "Pas de sauvegarde pour restaurer nginx."
    cp -a "$b/etc-nginx/nginx.conf" "$NGINX_CONF"
    if [[ -f "$b/etc-nginx/sites-available/${LEGACY_NAME}" ]]; then
        cp -a "$b/etc-nginx/sites-available/${LEGACY_NAME}" "$LEGACY_VHOST"
    fi
}

write_legacy_vhost() {
    cat > "$LEGACY_VHOST" <<'EOF'
# Vhost historique : ancien nom de l'API, conserve pour compatibilite.
# Aligne sur /etc/nginx/sites-available/relaisdesk-api (durcissement 2026-11-17).
# Certificat et server_name d'origine conserves.
server {
    listen 443 ssl http2;
    server_name api.informatiqueadomicile03.fr;

    ssl_certificate /etc/letsencrypt/live/informatiqueadomicile03.fr/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/informatiqueadomicile03.fr/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_timeout 1d;
    ssl_session_cache shared:RelaisDeskTLS:10m;
    ssl_session_tickets off;
    server_tokens off;

    client_max_body_size 11m;

    # Journal dedie : permet de mesurer l'usage reel de l'ancien domaine
    # (le journal global ne contient pas le nom d'hote).
    access_log /var/log/nginx/api-legacy.access.log;

    location /api/v1/downloads/ {
        proxy_pass https://127.0.0.1:8443;
        proxy_http_version 1.1;

        proxy_ssl_server_name on;
        proxy_ssl_name api.relaisdesk.fr;
        proxy_ssl_verify on;
        proxy_ssl_verify_depth 3;
        proxy_ssl_trusted_certificate /etc/ssl/certs/ca-certificates.crt;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header CF-Connecting-IP "";
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Connection "";

        proxy_buffering off;
        proxy_connect_timeout 10s;
        proxy_send_timeout 600s;
        proxy_read_timeout 600s;
    }

    location / {
        proxy_pass https://127.0.0.1:8443;
        proxy_http_version 1.1;

        # Le certificat de l'amont est emis pour api.relaisdesk.fr.
        proxy_ssl_server_name on;
        proxy_ssl_name api.relaisdesk.fr;
        proxy_ssl_verify on;
        proxy_ssl_verify_depth 3;
        proxy_ssl_trusted_certificate /etc/ssl/certs/ca-certificates.crt;

        # Remplacer les valeurs fournies par le client, ne jamais les concatener.
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header CF-Connecting-IP "";
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Connection "";

        # Delais d'envoi/lecture historiques conserves (connexions longues) ;
        # connexion amont a 5 s comme le vhost principal.
        proxy_connect_timeout 5s;
        proxy_send_timeout 60s;
        proxy_read_timeout 300s;
    }
}

# Redirection HTTP -> HTTPS
server {
    listen 80;
    server_name api.informatiqueadomicile03.fr;
    return 301 https://$host$request_uri;
}
EOF
}

cmd_nginx() {
    require_root
    require_backup
    [[ -f "$LEGACY_VHOST" ]] || die "Vhost introuvable : $LEGACY_VHOST"
    nginx -t 2>/dev/null || die "nginx -t echoue AVANT modification : corriger d'abord."

    log "Global : ssl_protocols TLSv1.2 TLSv1.3 et server_tokens off"
    # ssl_protocols (ligne active uniquement)
    if grep -qE '^[[:space:]]*ssl_protocols[[:space:]]' "$NGINX_CONF"; then
        sed -i -E 's/^([[:space:]]*)ssl_protocols[[:space:]].*;/\1ssl_protocols TLSv1.2 TLSv1.3;/' "$NGINX_CONF"
    else
        sed -i -E '0,/^[[:space:]]*include \/etc\/nginx\/mime\.types;/s//&\n\tssl_protocols TLSv1.2 TLSv1.3;/' "$NGINX_CONF"
    fi
    # server_tokens : decommenter si commente, sinon ajouter dans http{}
    if grep -qE '^[[:space:]]*server_tokens[[:space:]]+off;' "$NGINX_CONF"; then
        :
    elif grep -qE '^[[:space:]]*#[[:space:]]*server_tokens[[:space:]]+off;' "$NGINX_CONF"; then
        sed -i -E 's/^([[:space:]]*)#[[:space:]]*server_tokens[[:space:]]+off;/\1server_tokens off;/' "$NGINX_CONF"
    else
        sed -i -E '0,/^[[:space:]]*include \/etc\/nginx\/mime\.types;/s//&\n\tserver_tokens off;/' "$NGINX_CONF"
    fi

    log "Vhost historique ${LEGACY_NAME} : alignement sur ${MAIN_NAME}"
    write_legacy_vhost
    chmod 644 "$LEGACY_VHOST"

    if ! nginx -t; then
        err "nginx -t echoue : restauration automatique de la sauvegarde."
        restore_nginx_files
        nginx -t && ok "Configuration d'origine restauree (nginx non recharge)."
        exit 1
    fi

    systemctl reload nginx
    ok "nginx recharge."

    local c_main c_legacy
    c_main=$(curl_health "$MAIN_NAME")
    c_legacy=$(curl_health "$LEGACY_NAME")
    log "Sante ${MAIN_NAME} : ${c_main} ; ${LEGACY_NAME} : ${c_legacy}"
    if [[ "$c_main" != "200" || "$c_legacy" != "200" ]]; then
        err "Sante /api/v1/health != 200 : restauration automatique."
        restore_nginx_files
        nginx -t && systemctl reload nginx
        die "Etat d'origine restaure. Consulter /var/log/nginx/error.log."
    fi
    ok "Les deux domaines repondent 200."

    # Controles informatifs
    if command -v openssl >/dev/null 2>&1; then
        if echo | openssl s_client -connect 127.0.0.1:443 -servername "$LEGACY_NAME" -tls1_1 2>&1 \
            | grep -qE 'BEGIN CERTIFICATE|Protocol *: TLSv1\.1'; then
            warn "TLS 1.1 encore accepte sur ${LEGACY_NAME}."
        else
            ok "TLS 1.1 refuse sur ${LEGACY_NAME}."
        fi
    fi
    local srv
    srv=$(curl -sSI -m 10 --resolve "${LEGACY_NAME}:443:127.0.0.1" "https://${LEGACY_NAME}${HEALTH_PATH}" | grep -i '^server:' || true)
    log "En-tete ${srv:-Server: (absent)}"
    log "Mesure d'usage du domaine historique a venir : /var/log/nginx/api-legacy.access.log"
}

cmd_rollback_nginx() {
    require_root
    require_backup
    restore_nginx_files
    nginx -t || die "nginx -t echoue apres restauration : intervention manuelle."
    systemctl reload nginx
    ok "nginx.conf et vhost ${LEGACY_NAME} restaures depuis la sauvegarde, nginx recharge."
    log "Le journal /var/log/nginx/api-legacy.access.log (s'il existe) est conserve."
}

# --------------------------------------------------------------------------
# sshd
# --------------------------------------------------------------------------
cmd_sshd() {
    require_root
    require_backup
    # Garde-fous : un acces non-root doit exister avant d'interdire root.
    id ubuntu >/dev/null 2>&1 || die "Utilisateur 'ubuntu' absent : abandon."
    [[ -s /home/ubuntu/.ssh/authorized_keys ]] || die "Aucune cle pour 'ubuntu' : abandon (risque de perte d'acces)."
    sudo -n -u ubuntu sudo -n true 2>/dev/null || warn "ubuntu ne semble pas avoir sudo sans mot de passe."

    cat > "$SSHD_DROPIN" <<'EOF'
# Durcissement RelaisDesk 2026-11-17
X11Forwarding no
PermitRootLogin no
EOF
    chmod 644 "$SSHD_DROPIN"

    if ! sshd -t; then
        err "sshd -t echoue : retrait du drop-in."
        rm -f "$SSHD_DROPIN"
        exit 1
    fi
    systemctl reload ssh
    ok "sshd recharge."
    sshd -T | grep -Ei '^(permitrootlogin|x11forwarding|passwordauthentication) ' | sed 's/^/      /'

    local prl x11
    prl=$(sshd -T | awk '$1=="permitrootlogin"{print $2}')
    x11=$(sshd -T | awk '$1=="x11forwarding"{print $2}')
    [[ "$prl" == "no" && "$x11" == "no" ]] || warn "Valeurs effectives inattendues (permitrootlogin=$prl x11forwarding=$x11) : un autre fichier prime ?"

    warn "NE FERMEZ PAS cette session. Ouvrez MAINTENANT une 2e connexion :"
    warn "    ssh -i .secrets/oracle_openssh ubuntu@<ip>   puis   sudo -n true"
    warn "Si elle echoue, annulez ici : $0 rollback-sshd"
}

cmd_rollback_sshd() {
    require_root
    rm -f "$SSHD_DROPIN"
    sshd -t || die "sshd -t echoue : intervention manuelle."
    systemctl reload ssh
    ok "Drop-in supprime, sshd recharge (valeurs d'origine)."
    sshd -T | grep -Ei '^(permitrootlogin|x11forwarding) ' | sed 's/^/      /'
}

# --------------------------------------------------------------------------
# certbot
# --------------------------------------------------------------------------
cmd_certbot() {
    require_root
    require_backup
    [[ -x "$SNAP_CERTBOT" ]] || die "Certbot snap introuvable (${SNAP_CERTBOT}) : abandon, rien n'est modifie."
    log "Certbot snap : $($SNAP_CERTBOT --version 2>&1)"

    # Les certificats a authentificateur 'manual' (duckdns) echouent
    # toujours en dry-run : on teste uniquement les certificats nginx.
    # NB : l'authentificateur nginx modifie temporairement la config nginx
    # et recharge nginx pendant le test (coupure imperceptible).
    local names=() n auth f
    for f in /etc/letsencrypt/renewal/*.conf; do
        n=$(basename "$f" .conf)
        auth=$(awk -F' *= *' '$1=="authenticator"{print $2}' "$f")
        if [[ "$auth" == "manual" ]]; then
            warn "Certificat ${n} : authentificateur manual, ignore (non renouvelable automatiquement)."
        else
            names+=("$n")
        fi
    done
    [[ ${#names[@]} -gt 0 ]] || die "Aucun certificat renouvelable detecte."

    log "Dry-run avec le certbot snap sur : ${names[*]}"
    # `certbot renew` ne retient qu'un seul --cert-name : un dry-run par certificat.
    local i
    for i in "${names[@]}"; do
        if ! "$SNAP_CERTBOT" renew --dry-run --no-random-sleep-on-renew --cert-name "$i"; then
            die "Le dry-run snap a echoue pour $i : AUCUN changement effectue (cron et paquet apt conserves)."
        fi
    done
    ok "Dry-run snap reussi pour tous les certificats nginx."

    systemctl is-enabled snap.certbot.renew.timer >/dev/null 2>&1 \
        || die "snap.certbot.renew.timer n'est pas actif : abandon avant suppression de l'ancien mecanisme."
    systemctl is-active snap.certbot.renew.timer >/dev/null 2>&1 \
        || die "snap.certbot.renew.timer inactif : abandon."
    ok "Timer snap.certbot.renew.timer actif."
    [[ -x /etc/letsencrypt/renewal-hooks/deploy/relaisdesk.sh ]] \
        || die "Hook deploy relaisdesk.sh absent/non executable : abandon."

    # 1) ligne cron root (le hook deploy du repertoire s'execute deja seul
    #    a chaque renouvellement, quel que soit l'orchestrateur).
    if crontab -l -u root 2>/dev/null | grep -qE '(^|[^#])certbot[[:space:]]+renew'; then
        crontab -l -u root | grep -vE '^[^#]*certbot[[:space:]]+renew' | crontab -u root -
        ok "Ligne cron 'certbot renew' retiree du crontab root."
    else
        log "Aucune ligne cron 'certbot renew' (deja retiree)."
    fi

    # 2) timer apt
    if systemctl list-unit-files certbot.timer --no-legend 2>/dev/null | grep -q certbot.timer; then
        systemctl disable --now certbot.timer
        ok "certbot.timer (apt) desactive."
    fi

    # 3) paquet apt 'certbot' : on retire uniquement le binaire/timer apt.
    #    python3-certbot-nginx et python3-certbot sont des bibliotheques/plugin
    #    de ce certbot apt ; le snap embarque ses propres plugins nginx/apache
    #    et son propre Python, il n'en depend pas. On ne les retire pas
    #    (pas d'autoremove, pas de purge) : seule la CLI apt disparait,
    #    /etc/letsencrypt est conserve.
    if dpkg -s certbot >/dev/null 2>&1; then
        local removed
        removed=$(apt-get -s remove certbot | awk '/^Remv/{print $2}' | tr '\n' ' ')
        if [[ "${removed% }" != "certbot" ]]; then
            die "apt retirerait aussi : ${removed}. Abandon (cron/timer deja traites ; retirer le paquet a la main)."
        fi
        DEBIAN_FRONTEND=noninteractive apt-get remove -y certbot
        ok "Paquet apt 'certbot' retire (python3-certbot* conserves)."
    else
        log "Paquet apt 'certbot' deja absent."
    fi
    hash -r
    log "certbot resolu : $(command -v certbot || echo introuvable)"
    "$SNAP_CERTBOT" certificates 2>&1 | grep -E 'Certificate Name|Expiry' | sed 's/^/      /'
    systemctl list-timers --all --no-pager | grep -i certbot | sed 's/^/      /' || true
    warn "Le certificat ${DUCKDNS_CERT} (manual) n'est PAS renouvelable : voir 'certbot-delete-duckdns'."
}

cmd_certbot_delete_duckdns() {
    require_root
    require_backup
    [[ -x "$SNAP_CERTBOT" ]] || die "Certbot snap introuvable."
    "$SNAP_CERTBOT" certificates --cert-name "$DUCKDNS_CERT" 2>&1 | grep -q "Certificate Name: ${DUCKDNS_CERT}" \
        || { ok "Certificat ${DUCKDNS_CERT} deja absent."; return 0; }

    local refs
    refs=$(grep -rIl "$DUCKDNS_CERT" /etc/nginx /opt/relaisdesk /etc/systemd/system /etc/wireguard 2>/dev/null || true)
    if [[ -n "$refs" ]]; then
        err "Le certificat est reference ici :"
        while IFS= read -r l; do printf '      %s\n' "$l" >&2; done <<< "$refs"
        die "Suppression refusee."
    fi
    [[ -t 0 ]] || die "Terminal interactif requis pour confirmer."
    warn "Suppression DEFINITIVE du certificat ${DUCKDNS_CERT} (/etc/letsencrypt/{live,archive,renewal}/${DUCKDNS_CERT}*)."
    warn "Une copie existe dans $(oldest_backup)/etc-letsencrypt."
    local rep
    read -r -p "Tapez exactement SUPPRIMER pour confirmer : " rep
    [[ "$rep" == "SUPPRIMER" ]] || die "Annule."
    "$SNAP_CERTBOT" delete --cert-name "$DUCKDNS_CERT" --non-interactive
    ok "Certificat ${DUCKDNS_CERT} supprime."
}

cmd_rollback_certbot_cron() {
    require_root
    local b
    b=$(oldest_backup) || die "Pas de sauvegarde."
    crontab -u root "$b/root-crontab.txt"
    if dpkg -s certbot >/dev/null 2>&1; then :; else
        DEBIAN_FRONTEND=noninteractive apt-get install -y certbot
    fi
    systemctl enable --now certbot.timer || true
    ok "Crontab root restaure depuis $b ; certbot apt reinstalle si besoin."
    log "Le certificat supprime par certbot-delete-duckdns se restaure en recopiant $b/etc-letsencrypt/{live,archive,renewal}/${DUCKDNS_CERT}*"
}

# --------------------------------------------------------------------------
# iptables
# --------------------------------------------------------------------------
# Persistance : paquet netfilter-persistent/iptables-persistent, fichier
# /etc/iptables/rules.v4 recharge au boot. wg-quick@wg0 (PostUp) ajoute
# lui-meme `iptables -I INPUT -p udp --dport 51820 -j ACCEPT` a chaque
# demarrage : la regle appartient donc a WireGuard. On la retire du fichier
# persistant (sinon : N copies du fichier + 1 de PostUp apres chaque boot)
# et on n'en garde qu'UNE en live.
cmd_iptables() {
    require_root
    require_backup
    [[ -f "$IPT_RULES" ]] || die "Fichier de persistance introuvable : $IPT_RULES"
    local rule=(INPUT -p udp -m udp --dport 51820 -j ACCEPT)
    local n
    n=$(iptables -S INPUT | grep -cxF -- '-A INPUT -p udp -m udp --dport 51820 -j ACCEPT' || true)
    log "Regles live 51820/udp : ${n}"
    if [[ "$n" -eq 0 ]]; then
        warn "Aucune regle 51820/udp live : WireGuard est-il demarre ? Ajout d'une regle."
        iptables -I "${rule[@]}"
        n=1
    fi
    while [[ "$n" -gt 1 ]]; do
        iptables -D "${rule[@]}"
        n=$((n - 1))
    done
    ok "Regle 51820/udp live : 1."

    # Fichier persistant : conserve UNE seule occurrence. WireGuard ne depend
    # ainsi pas de l'ordre de demarrage netfilter-persistent / wg-quick ;
    # apres un boot on aura 2 regles (fichier + PostUp), sans consequence.
    local tmp
    tmp=$(mktemp)
    awk -v r='-A INPUT -p udp -m udp --dport 51820 -j ACCEPT' '$0 == r { if (seen++) next } { print }' "$IPT_RULES" > "$tmp"
    if iptables-restore --test < "$tmp"; then
        if ! cmp -s "$tmp" "$IPT_RULES"; then
            install -m 640 -o root -g root "$tmp" "$IPT_RULES"
            ok "${IPT_RULES} mis a jour (une seule regle 51820 conservee)."
        else
            log "${IPT_RULES} deja a jour."
        fi
    else
        rm -f "$tmp"
        die "Le fichier modifie est invalide : ${IPT_RULES} inchange."
    fi
    rm -f "$tmp"
    iptables -S INPUT | nl | sed 's/^/      /'
    warn "Non traites (hors perimetre) : regles nat POSTROUTING 'MASQUERADE -o ens3' dupliquees (meme cause : fichier + PostUp)."
}

cmd_rollback_iptables() {
    require_root
    local b
    b=$(oldest_backup) || die "Pas de sauvegarde."
    [[ -f "$b/etc-iptables/rules.v4" ]] || die "rules.v4 absent de la sauvegarde."
    install -m 640 -o root -g root "$b/etc-iptables/rules.v4" "$IPT_RULES"
    ok "Fichier ${IPT_RULES} restaure. Les regles live ne sont pas modifiees (une seule suffit)."
}

# --------------------------------------------------------------------------
# upgrade
# --------------------------------------------------------------------------
cmd_upgrade() {
    require_root
    require_backup
    export DEBIAN_FRONTEND=noninteractive
    export NEEDRESTART_MODE=l   # lister seulement, ne pas redemarrer de services
    apt-get update
    log "Paquets a mettre a jour :"
    apt list --upgradable 2>/dev/null | sed -n '2,$p' | sed 's/^/      /'
    if apt list --upgradable 2>/dev/null | grep -q '^docker-ce/'; then
        warn "docker-ce est mis a jour : le demon Docker redemarre, donc hbbs/hbbr"
        warn "(restart=unless-stopped) coupent quelques secondes. Faire cela en heure creuse."
    fi
    read_confirm_if_tty "Lancer apt-get upgrade ?" || die "Annule."
    apt-get -y -o Dpkg::Options::=--force-confold upgrade
    # dist-upgrade inutile : le noyau linux-oracle est un meta-paquet deja
    # a jour (linux-image-oracle 6.8.0-1067) ; 'upgrade' n'a rien retenu.
    local kept
    kept=$(apt-get -s upgrade | grep -c 'kept back' || true)
    [[ "$kept" -eq 0 ]] || warn "Des paquets sont retenus (kept back) : examiner 'apt-get -s dist-upgrade'."
    if [[ -f /var/run/reboot-required ]]; then
        warn "REDEMARRAGE NECESSAIRE : $(cat /var/run/reboot-required.pkgs 2>/dev/null | tr '\n' ' ')"
        warn "Voir : $0 reboot-and-check"
    else
        ok "Pas de redemarrage requis (noyau en cours : $(uname -r))."
    fi
    log "Services signales par needrestart (non redemarres) :"
    needrestart -b -r l 2>/dev/null | sed 's/^/      /' || true
    log "Enchainer avec : $0 verify"
}

read_confirm_if_tty() {
    [[ -t 0 ]] || return 0
    local r
    read -r -p "$1 [o/N] " r
    [[ "$r" == "o" || "$r" == "O" || "$r" == "y" || "$r" == "Y" ]]
}

cmd_reboot_and_check() {
    cat <<'EOF'
Redemarrage : JAMAIS automatique. Procedure :
  1. Prevenir : l'API et hbbs/hbbr seront indisponibles ~1 a 2 minutes
     (les clients RelaisDesk se reconnectent seuls).
  2. Verifier que 'verify' est OK et qu'une sauvegarde existe.
  3. Depuis le serveur :   sudo systemctl reboot
  4. Attendre ~90 s, se reconnecter (ssh ubuntu@...), puis :
        uptime ; uname -r
        sudo bash hardening-oracle-20261117.sh verify
  5. Tout est en 'enabled' (relaisdesk-api, nginx, docker, wg-quick@wg0,
     php8.1-fpm, mariadb, netfilter-persistent) et les conteneurs en
     restart=unless-stopped : tout doit revenir seul.
  6. Console serie / console Oracle Cloud en secours si SSH ne revient pas.
EOF
}

# --------------------------------------------------------------------------
# verify
# --------------------------------------------------------------------------
PASS=0; FAILS=0; WARNS=0
chk()  { if "${@:2}" >/dev/null 2>&1; then ok "$1"; PASS=$((PASS+1)); else err "$1"; FAILS=$((FAILS+1)); fi; }
chkw() { if "${@:2}" >/dev/null 2>&1; then ok "$1"; PASS=$((PASS+1)); else warn "$1"; WARNS=$((WARNS+1)); fi; }

listening() { # $1=proto(tcp|udp) $2=port
    ss -H -"${1:0:1}"ln | awk '{print $4}' | grep -qE "[:.]$2\$"
}

cmd_verify() {
    require_root
    log "Noyau : $(uname -r) ; uptime : $(uptime -p)"
    local u
    for u in relaisdesk-api nginx docker wg-quick@wg0 php8.1-fpm mariadb netfilter-persistent ssh; do
        chk "service actif : $u" systemctl is-active --quiet "$u"
        chkw "service enabled : $u" systemctl is-enabled --quiet "$u"
    done
    for u in rustdesk-hbbs rustdesk-hbbr; do
        chk "conteneur en marche : $u" bash -c "[ \"\$(docker inspect -f '{{.State.Running}}' $u)\" = true ]"
        chkw "restart policy unless-stopped : $u" bash -c "[ \"\$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' $u)\" = unless-stopped ]"
        chk "authentification exigee (RELAISDESK_AUTH_REQUIRED=Y) : $u" bash -c "docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' $u | grep -qx 'RELAISDESK_AUTH_REQUIRED=Y'"
        chk "cles publiques d'authentification presentes : $u" bash -c "docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' $u | grep -q '^RELAISDESK_AUTH_PUBLIC_KEYS=.\\+'"
    done
    local p
    for p in 80 443 21115 21116 21117; do chk "tcp/$p en ecoute" listening tcp "$p"; done
    chk "udp/21116 en ecoute" listening udp 21116
    chk "udp/51820 (WireGuard) en ecoute" listening udp 51820
    chk "API en ecoute sur 127.0.0.1:8443" bash -c "ss -H -tln | awk '{print \$4}' | grep -qx '127.0.0.1:8443'"

    local c
    c=$(curl_health "$MAIN_NAME");   chk "sante ${MAIN_NAME} = 200 (obtenu $c)" test "$c" = 200
    c=$(curl_health "$LEGACY_NAME"); chk "sante ${LEGACY_NAME} = 200 (obtenu $c)" test "$c" = 200
    local srv
    for u in "$MAIN_NAME" "$LEGACY_NAME"; do
        srv=$(curl -sSI -m 10 --resolve "${u}:443:127.0.0.1" "https://${u}${HEALTH_PATH}" | tr -d '\r' | grep -i '^server:' || true)
        chkw "en-tete Server sans version (${u} : ${srv:-absent})" bash -c "! echo '$srv' | grep -qE '[0-9]\\.[0-9]'"
    done
    chk "nginx -t" nginx -t
    chkw "nginx : ssl_protocols sans TLS 1.0/1.1" bash -c "! nginx -T 2>/dev/null | grep -E '^[[:space:]]*ssl_protocols' | grep -qE 'TLSv1([[:space:]]|;)|TLSv1\\.1'"

    local prl x11
    prl=$(sshd -T | awk '$1=="permitrootlogin"{print $2}')
    x11=$(sshd -T | awk '$1=="x11forwarding"{print $2}')
    chkw "sshd permitrootlogin=no (actuel : $prl)" test "$prl" = no
    chkw "sshd x11forwarding=no (actuel : $x11)" test "$x11" = no
    chk  "sshd passwordauthentication=no" bash -c "sshd -T | grep -qx 'passwordauthentication no'"

    local n
    n=$(iptables -S INPUT | grep -cxF -- '-A INPUT -p udp -m udp --dport 51820 -j ACCEPT' || true)
    chkw "iptables : regles 51820/udp en double (${n}, attendu 1)" test "$n" -le 1
    chk "iptables : REJECT final present" bash -c "iptables -S INPUT | tail -n1 | grep -q REJECT"

    if crontab -l -u root 2>/dev/null | grep -qE '^[^#]*certbot[[:space:]]+renew'; then
        warn "crontab root : ligne certbot renew encore presente (etape certbot non faite ?)"; WARNS=$((WARNS+1))
    fi
    chkw "timer snap.certbot.renew.timer actif" systemctl is-active --quiet snap.certbot.renew.timer
    chkw "pas de reboot en attente" test ! -f /var/run/reboot-required

    echo
    log "Resultat : ${PASS} OK, ${WARNS} avertissement(s), ${FAILS} echec(s)."
    [[ "$FAILS" -eq 0 ]] || exit 1
}

# --------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage : sudo bash $0 <sous-commande>

  backup                 Sauvegarde horodatee dans /root/hardening-${STAMP_DAY}-<ts>/
  nginx                  TLS1.2/1.3, server_tokens off, alignement du vhost historique
  sshd                   X11Forwarding no + PermitRootLogin no (drop-in)
  certbot                Dry-run snap, retrait cron + timer + paquet apt certbot
  certbot-delete-duckdns Suppression (confirmee) du certificat ${DUCKDNS_CERT}
  iptables               Dedoublonne 51820/udp (live + fichier persistant)
  upgrade                apt-get update && upgrade (pas de redemarrage)
  reboot-and-check       Procedure de redemarrage (affichage seulement)
  verify                 Controle complet de l'etat
  rollback-nginx | rollback-sshd | rollback-iptables | rollback-certbot-cron
EOF
}

main() {
    local sub="${1:-}"
    case "$sub" in
        backup)                 cmd_backup ;;
        nginx)                  cmd_nginx ;;
        sshd)                   cmd_sshd ;;
        certbot)                cmd_certbot ;;
        certbot-delete-duckdns) cmd_certbot_delete_duckdns ;;
        iptables)               cmd_iptables ;;
        upgrade)                cmd_upgrade ;;
        reboot-and-check)       cmd_reboot_and_check ;;
        verify)                 cmd_verify ;;
        rollback-nginx)         cmd_rollback_nginx ;;
        rollback-sshd)          cmd_rollback_sshd ;;
        rollback-iptables)      cmd_rollback_iptables ;;
        rollback-certbot-cron)  cmd_rollback_certbot_cron ;;
        ""|-h|--help|help)      usage ;;
        *) usage; exit 2 ;;
    esac
}

main "$@"
