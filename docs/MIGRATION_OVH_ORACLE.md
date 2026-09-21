# Migration RelaisDesk : site OVH, API et RustDesk sur Oracle

Mise à jour sécurité du 8 septembre 2026 : la migration de ce serveur est
déjà effectuée. Ne pas rejouer ce guide sur l'installation active ; consulter
[CORRECTIFS_SECURITE_2026-09-08.md](CORRECTIFS_SECURITE_2026-09-08.md) pour les
versions réellement déployées, les tests et les sauvegardes. Les modèles
précompilés ci-dessous utilisent désormais les binaires serveur corrigés.

Mise à jour du 7 septembre 2026 : la distribution n'est pas conditionnée à
SignPath. Voir [RECTIFICATIFS_JURIDIQUES_2026-09-07.md](RECTIFICATIFS_JURIDIQUES_2026-09-07.md)
pour la mise à jour coordonnée des CGV, de l'API et des installateurs. Le site
reste chez OVH ; la région OCI `eu-marseille-1` est conservée.

Ce document part de l'état réel suivant : le VPS Ubuntu Oracle exécute déjà
`hbbs`, `hbbr` et l'ancienne API. La migration ne déplace jamais le site web
principal hors d'OVH.

Ce document décrit la migration **technique**. Les prérequis contractuels,
RGPD, open source et consommateurs sont ordonnés dans le
[`GUIDE_JURIDIQUE_MIGRATION_PRODUCTION.md`](GUIDE_JURIDIQUE_MIGRATION_PRODUCTION.md).
Une migration privée et réversible de préproduction peut commencer avant la
fin de toutes les formalités. En revanche, la publication des forks, la bascule
publique, la première vente B2B et la première vente B2C ont chacune leur porte
juridique propre ; ne pas confondre recette privée et mise en production
commerciale.

## 1. Topologie finale impérative

| Nom ou port | Destination | Rôle |
|---|---|---|
| `relaisdesk.fr` | IP de l'hébergement web OVH | site principal |
| `www.relaisdesk.fr` | IP de l'hébergement web OVH | site principal |
| `api.relaisdesk.fr` | IP publique du VPS Oracle | API, `hbbs`, `hbbr` |
| `443/tcp` Oracle | Nginx Oracle | API seulement |
| `21115/tcp`, `21116/tcp+udp`, `21117/tcp` | VPS Oracle | RustDesk natif |
| `21118-21119/tcp` | VPS Oracle uniquement si client Web RustDesk | WebSocket ; fermés sinon |

Le certificat de `relaisdesk.fr` et `www.relaisdesk.fr` est géré par OVH. Le
certificat Oracle contient uniquement `api.relaisdesk.fr`. Le fichier Nginx
Oracle ne contient jamais `server_name relaisdesk.fr` ni
`server_name www.relaisdesk.fr`.

## 2. Conditions bloquantes avant la production du fork

Ne pas activer le contrôle réseau obligatoire tant que ces points ne sont pas
tous vrais :

1. les forks publics client, serveur et `hbb_common` donnent accès aux licences,
   modifications et sources correspondant aux binaires AGPL distribués, avec
   leurs scripts de construction. La licence des launchers indépendants doit
   être explicitement déterminée ; leur publication sous licence OSI n'est
   pas une condition générale de l'AGPL et relève notamment des conditions
   propres au programme SignPath envisagé ;
2. les clients Windows et Linux forkés ont été reconstruits après le passage à
   `api.relaisdesk.fr` ;
3. les licences et sources correspondantes sont accessibles, l'état de signature
   Windows est annoncé honnêtement et le manifeste Ed25519 correspond aux
   fichiers définitifs, qu'ils aient ou non une signature Authenticode ;
4. une copie restaurable de la base de l'ancienne API existe hors du VPS ;
5. la paire RustDesk existante `id_ed25519` / `id_ed25519.pub` a été sauvegardée ;
6. la nouvelle API et le serveur forké ont été testés avec
   `RELAISDESK_AUTH_REQUIRED=N`, sans clé publique chargée ;
7. une plage de maintenance a été annoncée pour le passage final à `Y`.
8. les portes juridiques A et B du guide juridique sont closes ; la porte C est
   également close avant toute vente B2B, et la porte D avant toute vente B2C.

Les exécutables déjà présents dans `relaisdesk/downloads` sont antérieurs au
changement de domaine jusqu'à leur reconstruction. Ne pas les publier tels
quels en production.

### Déploiement commercial par jalons

Le calendrier peut avancer en parallèle, mais la mise en vente dépend de
critères vérifiables plutôt que d'un numéro de semaine :

| Jalon | Ce qui peut être fait | Critère de sortie |
|---|---|---|
| API commerciale, serveur Community conservé | Déployer la nouvelle API RelaisDesk sur `api.relaisdesk.fr`, sans remplacer immédiatement les `hbbs`/`hbbr` existants | HTTPS, sauvegarde/restauration, SMTP, Stripe en mode production, téléchargement signé, sources AGPL correspondant exactement aux binaires distribués et parcours d'achat testés de bout en bout |
| Fork en compatibilité | Déployer les `hbbs`/`hbbr` forkés avec `RELAISDESK_AUTH_REQUIRED=N` **et** `RELAISDESK_AUTH_PUBLIC_KEYS=` vide | anciens launchers et nouveaux clients passent la recette ; forks AGPL et code source correspondant publiés |
| Contrôle réseau obligatoire | Activer la clé publique et `RELAISDESK_AUTH_REQUIRED=Y` pendant une maintenance annoncée | recette d'au moins cinq minutes validée après bascule, nouveaux clients disponibles et retour arrière préparé |

La première vente B2B peut intervenir au premier jalon si les documents
contractuels, la facturation, Stripe, le courriel et les téléchargements sont
réellement opérationnels. Cette phase n'apporte toutefois pas encore le verrou
anti-partage du fork : ne pas annoncer cette fonction comme active avant le
troisième jalon. Une vente B2C reste bloquée tant que le médiateur de la
consommation et les autres prérequis consommateurs ne sont pas renseignés ;
conserver `B2C_SALES_ENABLED=false` jusque-là.

## 3. Préparer le poste Windows

Ouvrir PowerShell dans la racine du projet :

```powershell
Set-Location 'C:\Users\Administrator\Documents\Projets\projet'
git --version
gh --version
go version
cargo --version
```

Conserver une copie hors ligne du dossier avant les opérations Git. Ne jamais
inclure dans un dépôt ou une archive publique `api/.env`, `licenseAdmin.txt`,
`dns token(important).txt`, une clé privée, une base de données ou le dossier
`rustdesk-server/data`.

Ajouter les dépôts locaux à la liste sûre uniquement si Git affiche l'erreur
`dubious ownership` :

```powershell
git config --global --add safe.directory 'C:/Users/Administrator/Documents/Projets/projet/rustdesk'
git config --global --add safe.directory 'C:/Users/Administrator/Documents/Projets/projet/rustdesk-server'
git config --global --add safe.directory 'C:/Users/Administrator/Documents/Projets/projet/rustdesk/libs/hbb_common'
git config --global --add safe.directory 'C:/Users/Administrator/Documents/Projets/projet/rustdesk-server/libs/hbb_common'
```

## 4. Publier proprement les trois forks AGPL

S'authentifier sans coller de jeton dans un fichier :

```powershell
gh auth login
$GitHubUser = gh api user --jq .login
```

Créer les forks publics :

```powershell
gh repo fork rustdesk/hbb_common --clone=false
gh repo fork rustdesk/rustdesk --clone=false
gh repo fork rustdesk/rustdesk-server --clone=false
```

Les deux copies locales de `hbb_common` ont le même commit protocolaire mais
possèdent actuellement des adaptations non validées différentes. Les publier
sur deux branches du même fork, après revue :

```powershell
git -C .\rustdesk-server\libs\hbb_common switch -c relaisdesk/server-protocol
git -C .\rustdesk-server\libs\hbb_common diff --check
git -C .\rustdesk-server\libs\hbb_common add -A
git -C .\rustdesk-server\libs\hbb_common commit -m 'chore(relaisdesk): finalize server protocol dependencies'
git -C .\rustdesk-server\libs\hbb_common remote add relaisdesk "https://github.com/$GitHubUser/hbb_common.git"
git -C .\rustdesk-server\libs\hbb_common push -u relaisdesk relaisdesk/server-protocol

git -C .\rustdesk\libs\hbb_common switch -c relaisdesk/client-protocol
git -C .\rustdesk\libs\hbb_common diff --check
git -C .\rustdesk\libs\hbb_common add -A
git -C .\rustdesk\libs\hbb_common commit -m 'chore(relaisdesk): finalize client protocol dependencies'
git -C .\rustdesk\libs\hbb_common remote add relaisdesk "https://github.com/$GitHubUser/hbb_common.git"
git -C .\rustdesk\libs\hbb_common push -u relaisdesk relaisdesk/client-protocol
```

Si la branche existe déjà, utiliser `switch` sans `-c`. Si le remote existe
déjà, utiliser `git remote set-url relaisdesk ...`. Ne jamais utiliser
`push --force` pour résoudre ce cas.

Les fichiers `.gitmodules` utilisent `../hbb_common` : GitHub Actions résoudra
donc automatiquement le sous-module vers le fork du même propriétaire. Mettre
à jour et publier les deux dépôts parents :

```powershell
git -C .\rustdesk diff --check
git -C .\rustdesk status --short
git -C .\rustdesk add -A
git -C .\rustdesk commit -m 'feat(relaisdesk): finalize authorized client fork'
git -C .\rustdesk remote add relaisdesk "https://github.com/$GitHubUser/rustdesk.git"
git -C .\rustdesk push -u relaisdesk relaisdesk/authorization

git -C .\rustdesk-server diff --check
git -C .\rustdesk-server status --short
git -C .\rustdesk-server add -A
git -C .\rustdesk-server commit -m 'feat(relaisdesk): finalize authorized server fork'
git -C .\rustdesk-server remote add relaisdesk "https://github.com/$GitHubUser/rustdesk-server.git"
git -C .\rustdesk-server push -u relaisdesk relaisdesk/authorization
```

Avant chaque `add -A`, relire `status` et `diff`; ces dépôts contiennent des
modifications de travail qui ne doivent pas être écrasées.

## 5. Reconstruire et signer les clients

Pour une recette Windows locale non signée :

```powershell
.\scripts\build-rustdesk-windows.ps1
```

Pour les artefacts Flutter Windows et Linux, lancer le workflow du fork après
le push, puis télécharger les artefacts du run réussi :

```powershell
gh workflow run flutter-nightly.yml --repo "$GitHubUser/rustdesk" --ref relaisdesk/authorization
gh run list --repo "$GitHubUser/rustdesk" --workflow flutter-nightly.yml --limit 5
$RunId = 'REMPLACER_PAR_RUN_ID'
gh run watch $RunId --repo "$GitHubUser/rustdesk" --exit-status
$Artifacts = "C:\build\relaisdesk-ci\run-$RunId"
New-Item -ItemType Directory -Force $Artifacts | Out-Null
gh run download $RunId --repo "$GitHubUser/rustdesk" --name rustdesk-unsigned-windows-x86_64 --dir "$Artifacts\windows-raw"
gh run download $RunId --repo "$GitHubUser/rustdesk" --name rustdesk-windows-packages-x86_64 --dir "$Artifacts\windows-packages"
gh run download $RunId --repo "$GitHubUser/rustdesk" --name rustdesk-1.4.9-x86_64.deb --dir "$Artifacts\linux-deb"
gh run download $RunId --repo "$GitHubUser/rustdesk" --name rustdesk-sbom --dir "$Artifacts\sbom"
```

Ce workflow produit actuellement un artefact **non signé**. Il peut être
distribué avec l'information correspondante, les licences et sources requises
et un manifeste Ed25519 valide. La signature Authenticode via SignPath reste
une option à préparer selon [`SIGNPATH.md`](SIGNPATH.md).

Extraire le véritable binaire ELF du paquet Debian avant de construire les
launchers Linux. Avec 7-Zip installé :

```powershell
$Deb = Get-ChildItem -LiteralPath "$Artifacts\linux-deb" -Filter *.deb | Select-Object -First 1
$DebExtract = "$Artifacts\linux-deb-extracted"
New-Item -ItemType Directory -Force $DebExtract | Out-Null
7z x $Deb.FullName "-o$DebExtract" -y
$DataArchive = Get-ChildItem -LiteralPath $DebExtract -Filter 'data.tar.*' | Select-Object -First 1
7z x $DataArchive.FullName "-o$DebExtract" -y
$DataTar = Join-Path $DebExtract 'data.tar'
7z x $DataTar "-o$DebExtract\root" -y
$LinuxBinary = Join-Path $DebExtract 'root\usr\share\rustdesk\rustdesk'
if (-not (Test-Path -LiteralPath $LinuxBinary -PathType Leaf)) { throw "Binaire ELF introuvable dans le DEB" }
Get-Item -LiteralPath $LinuxBinary
```

Le launcher Windows doit embarquer l'exécutable portable auto-extractible du
dossier `windows-packages`, et non le petit lanceur Flutter isolé du dossier
`windows-raw` qui dépend de ses DLL et de son répertoire `data`. Avant toute
utilisation, sélectionner ce paquet, calculer les empreintes et confirmer qu'il
est bien encore non signé :

```powershell
$WindowsBinary = Get-ChildItem -LiteralPath "$Artifacts\windows-packages" -Filter *.exe | Select-Object -First 1
if ($null -eq $WindowsBinary) { throw "Exécutable portable Windows introuvable" }
Get-AuthenticodeSignature -LiteralPath $WindowsBinary | Format-List Path,Status,StatusMessage
Get-ChildItem -LiteralPath $Artifacts -Recurse -File |
  Get-FileHash -Algorithm SHA256 |
  Sort-Object Path |
  Format-Table Hash,Path -AutoSize
```

Le statut Authenticode attendu ici est `NotSigned`. Un statut `Valid` ne sera
exigé qu'après l'intégration SignPath. Ne jamais mélanger des artefacts issus
de deux identifiants de run ou de deux commits différents.

Générer une seule fois la clé privée de signature de version :

```powershell
New-Item -ItemType Directory -Force C:\RelaisDesk-Secrets | Out-Null
Set-Location .\api
go run ./cmd/release-keygen -out C:\RelaisDesk-Secrets\release-signing-ed25519
Set-Location ..
```

Sauvegarder cette clé privée hors ligne. La commande suivante reste une recette
locale jusqu'à l'intégration SignPath :

```powershell
Set-Location .\installer
.\build.ps1 `
  -RustDeskForkWindowsPath "$($WindowsBinary.FullName)" `
  -RustDeskForkLinuxDebPath "$($Deb.FullName)" `
  -RustDeskForkLinuxBinaryPath "$LinuxBinary" `
  -ReleaseSigningKeyPath C:\RelaisDesk-Secrets\release-signing-ed25519 `
  -ReleasePublicKey K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo
Set-Location ..
```

Le script refuse un faux PE, un faux DEB ou un ELF d'une autre architecture et
ne signe dans le manifeste que les six sorties effectivement référencées par
le site. Un fichier ancien laissé dans `downloads` ne doit jamais être repris
implicitement dans `SHA256SUMS.txt` ou `release-manifest.json`.

Ne pas acheter de certificat EV. Après acceptation du dossier, SignPath conserve
la clé Authenticode dans son HSM et signe depuis la CI GitHub publique. Windows
affichera `SignPath Foundation` comme éditeur. La chaîne doit obligatoirement
signer le fork RustDesk avant de l'embarquer, signer ensuite les launchers,
construire NSIS avec ces fichiers signés, signer les installateurs, puis
recalculer en dernier `SHA256SUMS.txt` et le manifeste Ed25519. Le
`release-manifest.json` produit avant une modification Authenticode ne doit pas
être réutilisé après celle-ci, car les octets et les SHA-256 changent. Sans
Authenticode, le manifeste doit correspondre aux fichiers non signés définitifs.

Vérifier récursivement les signatures et les empreintes des fichiers définitifs
téléchargés depuis le workflow :

```powershell
Get-ChildItem C:\build\relaisdesk-signed -Recurse -Include *.exe,*.dll |
  Get-AuthenticodeSignature |
  Format-Table Path,Status,SignerCertificate
Get-FileHash C:\build\relaisdesk-signed\* -Algorithm SHA256
```

Tous les statuts doivent être `Valid`. Azure Artifact Signing reste le premier
repli en cas de refus définitif de SignPath, puis vient un certificat OV. Une
signature valide ne garantit pas la disparition immédiate de SmartScreen.

## 6. Préparer le DNS sans déplacer le site

Dans la zone DNS OVH de `relaisdesk.fr` :

- `A relaisdesk.fr` vers l'IP de l'hébergement web OVH ;
- `A www.relaisdesk.fr` vers l'IP de l'hébergement web OVH ;
- `A api.relaisdesk.fr` vers l'IP publique du VPS Oracle.

Vérifier depuis Windows :

```powershell
Resolve-DnsName relaisdesk.fr -Type A
Resolve-DnsName www.relaisdesk.fr -Type A
Resolve-DnsName api.relaisdesk.fr -Type A
```

Les deux premières réponses doivent être l'IP OVH ; la dernière doit être
l'IP Oracle.

## 7. Inventorier l'Oracle existant avant toute coupure

Se connecter :

```powershell
ssh <UTILISATEUR_UBUNTU>@<IP_ORACLE>
```

Sur Ubuntu, enregistrer l'état :

```bash
sudo ss -lntup
sudo systemctl list-units --type=service --all | grep -Ei 'rustdesk|hbbs|hbbr|relaisdesk|api|nginx'
sudo docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'
sudo nginx -T
sudo find / -xdev -type f \( -name 'id_ed25519' -o -name 'id_ed25519.pub' -o -name 'db_v2.sqlite3' -o -name 'licences.db' \) 2>/dev/null
```

Noter dans un fichier local non public :

- le nom exact de l'ancienne unité API ;
- le chemin exact de son fichier d'environnement et de sa base SQLite ;
- si `hbbs` et `hbbr` sont lancés par Docker ou systemd ;
- le répertoire qui contient `id_ed25519`, `id_ed25519.pub` et
  `db_v2.sqlite3` ;
- la sortie des montages Docker si les services sont conteneurisés :

```bash
sudo docker inspect hbbs --format '{{json .Mounts}}'
sudo docker inspect hbbr --format '{{json .Mounts}}'
```

## 8. Sauvegarder correctement l'existant

Créer un dossier daté et restrictif :

```bash
sudo install -d -m 0700 /root/relaisdesk-before-migration
```

Sauvegarder SQLite avec son mécanisme de sauvegarde, pas avec un simple `cp`
pendant que l'API écrit :

```bash
sudo sqlite3 <CHEMIN_ANCIENNE_BASE> ".backup '/root/relaisdesk-before-migration/licences.db'"
sudo sqlite3 /root/relaisdesk-before-migration/licences.db 'PRAGMA integrity_check;'
```

La réponse attendue est `ok`. Sauvegarder l'identité RustDesk et sa base :

```bash
sudo cp -a <ANCIEN_DOSSIER_RUSTDESK>/id_ed25519 /root/relaisdesk-before-migration/
sudo cp -a <ANCIEN_DOSSIER_RUSTDESK>/id_ed25519.pub /root/relaisdesk-before-migration/
sudo cp -a <ANCIEN_DOSSIER_RUSTDESK>/db_v2.sqlite3 /root/relaisdesk-before-migration/
```

Copier ensuite cette sauvegarde chiffrée vers un support hors du VPS. Ne pas
continuer si `integrity_check` échoue ou si la clé privée RustDesk est absente.

## 9. Installer les prérequis Ubuntu et transférer le projet

```bash
sudo apt update
sudo apt install -y nginx certbot python3-certbot-nginx sqlite3 git curl ca-certificates build-essential
sudo systemctl enable --now nginx
```

Ce VPS utilise déjà `iptables` en plus du pare-feu Oracle : ne pas installer ni
activer UFW pendant la migration.

Vérifier d'abord l'installation Docker existante, car l'ancien `hbbs`/`hbbr`
peut déjà en dépendre :

```bash
sudo docker version
sudo docker compose version
```

Si Docker fonctionne, ne pas le remplacer au cours de cette migration. S'il
n'est pas installé, suivre le dépôt APT officiel Docker puis installer
`docker-ce docker-ce-cli containerd.io docker-buildx-plugin
docker-compose-plugin`. Vérifier ensuite :

```bash
sudo systemctl enable --now docker
sudo docker run --rm hello-world
sudo docker compose version
```

Le VPS inventorié est un `x86_64` doté de 1 Gio de RAM. Ne pas y compiler le
fork Rust ni tenter d'utiliser son Go 1.23.1 pour le module Go 1.26.6. Le
paquet AMD64 préparé localement contient les binaires API Go statiques et les
binaires serveur validés par la CI. Il ne contient aucun `.env`, base, clé ou
certificat.

Depuis Windows, vérifier puis transférer ce paquet :

```powershell
Set-Location 'C:\Users\Administrator\Documents\Projets\projet'
$expected = 'C2A3350CC887D665E1C1CEE3E1BFCB92149F4E945434672764515979EC23960E'
$archive = '.cache\relaisdesk-oracle-deploy-amd64-20260903.tar.gz'
if ((Get-FileHash $archive -Algorithm SHA256).Hash -ne $expected) {
    throw 'Empreinte du paquet Oracle invalide'
}
scp $archive <UTILISATEUR_UBUNTU>@<IP_ORACLE>:/tmp/
```

Puis sur Oracle :

```bash
sudo install -d -m 0750 -o "$USER" -g "$USER" /srv/relaisdesk-src
echo 'c2a3350cc887d665e1c1cee3e1bfcb92149f4e945434672764515979ec23960e  /tmp/relaisdesk-oracle-deploy-amd64-20260903.tar.gz' | sha256sum --check
tar -xzf /tmp/relaisdesk-oracle-deploy-amd64-20260903.tar.gz -C /srv/relaisdesk-src
cd /srv/relaisdesk-src
sudo bash ./scripts/setup_server.sh
bash -n scripts/*.sh
(cd prebuilt/api && sha256sum --check SHA256SUMS.txt)
(cd rustdesk && sha256sum --check SHA256SUMS.txt)
```

## 10. Créer les trois clés sans les confondre

La clé `id_ed25519` est l'identité RustDesk historique : la conserver. Ne pas
la faire tourner pendant cette migration.

Créer sur Oracle la clé d'autorisation réseau de l'API :

```bash
/srv/relaisdesk-src/prebuilt/api/network-keygen -out /tmp/network-auth-ed25519
sudo install -m 0600 -o relaisdesk -g relaisdesk /tmp/network-auth-ed25519 /etc/relaisdesk/network-auth-ed25519
rm /tmp/network-auth-ed25519
```

Conserver la clé publique base64url affichée pour
`RELAISDESK_AUTH_PUBLIC_KEYS`. La troisième clé est la clé de version créée sur
Windows ; sa partie privée ne doit jamais arriver sur Oracle.

## 11. Préparer le certificat Oracle

Le DNS de l'API doit déjà pointer vers Oracle et le port 80 doit être
autorisé. Installer d'abord le bloc HTTP temporaire :

```bash
sudo install -m 0644 /srv/relaisdesk-src/scripts/nginx-api-http-bootstrap.conf.example /etc/nginx/sites-available/relaisdesk-api
sudo ln -sfn /etc/nginx/sites-available/relaisdesk-api /etc/nginx/sites-enabled/relaisdesk-api
sudo nginx -t
sudo systemctl reload nginx
```

Obtenir ensuite un certificat nommé de façon stable :

```bash
sudo certbot certonly --nginx --cert-name api.relaisdesk.fr \
  -d api.relaisdesk.fr
sudo openssl x509 -in /etc/letsencrypt/live/api.relaisdesk.fr/fullchain.pem -noout -text | grep -A1 'Subject Alternative Name'
```

Installer le hook qui fournit une copie lisible uniquement par l'API :

```bash
sudo install -D -m 0755 /srv/relaisdesk-src/scripts/certbot-deploy.sh /etc/letsencrypt/renewal-hooks/deploy/relaisdesk.sh
sudo env RELAISDESK_SKIP_SERVICE_RESTART=1 /etc/letsencrypt/renewal-hooks/deploy/relaisdesk.sh
```

La variable temporaire empêche le hook initial de redémarrer l'ancienne API
avec le nouveau fichier d'environnement. Les renouvellements Certbot suivants,
qui n'emploient pas cette variable, redémarreront normalement la nouvelle API.

## 12. Migrer et configurer la nouvelle API

Installer les binaires et unités sans démarrer l'API :

```bash
cd /srv/relaisdesk-src
sudo bash ./scripts/deploy_api_prebuilt.sh
```

Éditer `/etc/relaisdesk/api.env`. Les valeurs structurantes sont :

```dotenv
API_BIND=127.0.0.1
API_PORT=8443
DB_PATH=/data/relaisdesk/licences.db
SERVER_IP=api.relaisdesk.fr
RUSTDESK_RENDEZVOUS_PORT=21116
RUSTDESK_RELAY_PORT=21117
TLS_CERT=/opt/relaisdesk/certs/fullchain.pem
TLS_KEY=/opt/relaisdesk/certs/privkey.pem
DEV_HTTP=false
NETWORK_AUTH_PRIVATE_KEY_FILE=/etc/relaisdesk/network-auth-ed25519
NETWORK_AUTH_KEY_ID=relaisdesk-1
NETWORK_TOKEN_TTL_SECONDS=300
PUBLIC_WEBSITE_URL=https://relaisdesk.fr
CLIENT_SOURCE_URL=https://github.com/JuLeXoGame/relaisdesk/tree/e34c8bab932791961b2406bac5d0256b744b40b1
SERVER_SOURCE_URL=https://github.com/JuLeXoGame/rustdesk-server/tree/d998a326afc32bbd26614e84f2b3332e8fdd10ab
RELEASE_MANIFEST_PATH=/opt/relaisdesk/downloads/release-manifest.json
RELEASE_PUBLIC_KEY=K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo
LEGAL_PHONE=06 62 85 59 30
B2C_SALES_ENABLED=false
```

Renseigner aussi un `ADMIN_TOKEN` aléatoire d'au moins 32 caractères, SMTP,
Stripe si utilisé et les vraies coordonnées bancaires. Générer le jeton avec
`openssl rand -base64 48`; ne jamais le copier dans une commande publiée.

### Configurer le webhook Stripe dans le Dashboard

Le chemin réellement exposé par cette API est :

```text
https://api.relaisdesk.fr/api/v1/stripe/webhook
```

Ne pas utiliser `/api/v1/webhooks/stripe`, qui n'existe pas dans le routeur.
Dans Stripe Workbench, créer une destination HTTPS pour le compte RelaisDesk,
sélectionner l'événement `checkout.session.completed`, puis copier le secret
`whsec_...` de **cette destination de production** dans
`STRIPE_WEBHOOK_SECRET`. Le secret affiché par Stripe CLI ou par une destination
de test n'est pas interchangeable avec celui de production. Configurer en même
temps `STRIPE_SECRET_KEY`; l'API de production refuse une configuration Stripe
partielle.

Avant une vente réelle :

1. envoyer un événement de test et confirmer une réponse HTTP `2xx` dans
   Stripe ;
2. effectuer un achat contrôlé en mode production ;
3. vérifier dans RelaisDesk la commande payée, la création ou le renouvellement
   de licence, la facture et le courriel ;
4. vérifier qu'une nouvelle livraison du même événement ne crée aucun doublon ;
5. désactiver l'ancienne destination uniquement après validation de la nouvelle.

Le retour du navigateur vers la page de succès ne remplace pas le webhook : le
client peut fermer la page avant le traitement. Le Checkout actuel limite les
moyens de paiement à `card`. Avant d'ajouter un moyen asynchrone, implémenter et
tester aussi l'événement Stripe de confirmation différée correspondant ; le
handler actuel ignore les autres types d'événements.

Référence : [configuration officielle des webhooks
Stripe](https://docs.stripe.com/webhooks).

### Vérifier les liens des courriels

Les templates présents utilisent déjà `https://relaisdesk.fr`,
`https://relaisdesk.fr/client/` et les téléchargements sous
`https://api.relaisdesk.fr`. Refaire néanmoins ce contrôle avant chaque mise en
production :

```bash
cd /srv/relaisdesk-src
grep -REnE "https?://(api\.)?informatiqueadomicile03\.fr|https?://relaist\.cluster129\.hosting\.ovh\.net" api/mailer
grep -REnE "https://(api\.)?relaisdesk\.fr" api/mailer
```

La première commande ne doit retourner aucun lien public historique. Un nom
d'hôte SMTP, une adresse d'expéditeur ou un test sans URL n'est pas un lien
client et peut légitimement rester sur le domaine de messagerie existant.

Tester la migration sur une copie de la base avant la coupure :

```bash
sudo sqlite3 /data/relaisdesk/licences.db \
  ".backup '/data/relaisdesk/licences-migration-test.db'"
sudo chown relaisdesk:relaisdesk /data/relaisdesk/licences-migration-test.db
sudo chmod 0600 /data/relaisdesk/licences-migration-test.db
```

Exécuter la migration et les contrôles sur cette copie :

```bash
sudo -u relaisdesk /opt/relaisdesk/api/relaisdesk-dbcheck \
  -db /data/relaisdesk/licences-migration-test.db
```

La réponse attendue est `Migration et intégrité SQLite : OK`. Ne remplacer la
base de production qu'après ce test.

Lors de la coupure de l'ancienne API :

```bash
sudo systemctl stop relaisdesk-api
sudo sqlite3 /data/relaisdesk/licences.db \
  'PRAGMA wal_checkpoint(TRUNCATE); PRAGMA integrity_check;'
sudo chown relaisdesk:relaisdesk /data/relaisdesk/licences.db
sudo chmod 0600 /data/relaisdesk/licences.db
```

Copier les artefacts signés et le manifeste de Windows vers
`/opt/relaisdesk/downloads`, avec des fichiers `0640 root:relaisdesk`.

## 13. Configurer Nginx Oracle pour l'API seulement

```bash
sudo install -m 0644 /srv/relaisdesk-src/scripts/nginx-api-relaisdesk.conf.example /etc/nginx/sites-available/relaisdesk-api
sudo ln -sfn /etc/nginx/sites-available/relaisdesk-api /etc/nginx/sites-enabled/relaisdesk-api
sudo nginx -t
sudo systemctl reload nginx
```

Supprimer ou désactiver uniquement les anciens blocs Oracle qui revendiquent
les mêmes noms d'API. Vérifier :

```bash
sudo nginx -T | grep -n 'server_name'
```

La sortie Oracle ne doit contenir aucun bloc pour `relaisdesk.fr` ou `www`.
Démarrer l'API :

```bash
sudo systemctl enable --now relaisdesk-api
sudo systemctl status relaisdesk-api --no-pager
curl --fail --show-error https://api.relaisdesk.fr/api/v1/health
```

Cette URL doit joindre la nouvelle API à travers Nginx.

## 14. Basculer `hbbs` et `hbbr` sans changer leur identité

Préparer l'image légère avant la coupure. Elle utilise les binaires AMD64 issus
de la CI et une base Ubuntu 24.04 officielle figée par digest, car `hbbs`
requiert glibc 2.39. Aucun compilateur Rust n'est exécuté sur le VPS :

Pour cette version, utiliser l'artefact `relaisdesk-server-linux-amd64` du
[run serveur 34169496112](https://github.com/JuLeXoGame/rustdesk-server/actions/runs/34169496112),
commit `d998a326afc32bbd26614e84f2b3332e8fdd10ab`. Le Dockerfile contrôle les
empreintes de ces deux binaires : changer uniquement le tag ou l'étiquette
de provenance ne suffit pas. Copier les modèles actualisés de `scripts/`
avant d'appliquer les commandes suivantes sur une nouvelle installation.

```bash
cd /srv/relaisdesk-src/rustdesk
sha256sum --check SHA256SUMS.txt
sudo install -m 0644 /srv/relaisdesk-src/scripts/docker-compose-oracle-prebuilt.yml /opt/rustdesk/docker-compose.yml.new
sudo install -m 0644 /srv/relaisdesk-src/scripts/Dockerfile.relaisdesk-prebuilt-amd64 /opt/rustdesk/Dockerfile.relaisdesk-prebuilt-amd64
sudo install -m 0644 /srv/relaisdesk-src/scripts/dockerignore-relaisdesk-prebuilt.example /opt/rustdesk/.dockerignore
sudo install -m 0755 hbbs /opt/rustdesk/hbbs
sudo install -m 0755 hbbr /opt/rustdesk/hbbr
```

Créer `/opt/rustdesk/.env` avec la clé publique produite à l'étape 10. Comme
aucun ancien client n'est à préserver, l'autorisation obligatoire est activée
dès la bascule :

```dotenv
RELAISDESK_AUTH_REQUIRED=Y
RELAISDESK_AUTH_PUBLIC_KEYS=relaisdesk-1=<CLE_PUBLIQUE_AUTORISATION_RESEAU>
SERVER_SOURCE_URL=https://github.com/JuLeXoGame/rustdesk-server/tree/d998a326afc32bbd26614e84f2b3332e8fdd10ab
```

Protéger le fichier puis valider la configuration qui remplacera l'ancienne :

```bash
sudo chown root:root /opt/rustdesk/.env
sudo chmod 0600 /opt/rustdesk/.env
cd /opt/rustdesk
sudo docker compose -f docker-compose.yml.new config --quiet
sudo docker build --pull -f Dockerfile.relaisdesk-prebuilt-amd64 -t relaisdesk/rustdesk-server:d998a32-security-amd64 .
sudo docker run --rm --entrypoint hbbs relaisdesk/rustdesk-server:d998a32-security-amd64 --help >/dev/null
sudo docker run --rm --entrypoint hbbr relaisdesk/rustdesk-server:d998a32-security-amd64 --help >/dev/null
```

Avant la coupure, vérifier dans la sortie normalisée que le nouveau Compose
publie bien TCP `21115`, TCP/UDP `21116` et TCP `21117`, mais pas les ports Web
`21118-21119` :

```bash
sudo docker compose -f docker-compose.yml.new config | grep -A12 'ports:'
```

Arrêter les anciennes instances, conserver leur Compose pour le retour arrière,
donner au nouvel UID conteneur l'accès au volume historique, puis démarrer :

```bash
cd /opt/rustdesk
sudo cp -a docker-compose.yml docker-compose.yml.before-fork
sudo docker compose down
sudo chown -R 10001:10001 data
sudo chmod 0700 data
sudo chmod 0600 data/id_ed25519
sudo mv docker-compose.yml.new docker-compose.yml
sudo docker compose up -d --no-build --pull never
sudo docker compose ps
sudo docker compose logs --tail=100 hbbs hbbr
sudo ss -lntup | grep -E ':(21115|21116|21117)\b'
```

À ce moment, un client RustDesk officiel ou un ancien launcher qui embarque ce
client ne peut plus ouvrir une nouvelle connexion.

## 15. Pare-feu Ubuntu et règles Oracle Cloud

Le VPS inventorié utilise des règles `iptables` existantes. Ne pas exécuter
`scripts/firewall.sh` et ne pas activer UFW : empiler un second gestionnaire
pourrait modifier l'ordre des chaînes ou couper SSH. Avant la bascule,
contrôler au minimum :

```bash
sudo iptables --version
sudo iptables -S INPUT
sudo iptables -S DOCKER-USER
sudo ip6tables -S INPUT
sudo ip6tables -S DOCKER-USER
sudo systemctl is-enabled netfilter-persistent
```

Une chaîne absente peut produire une erreur sans signifier que le pare-feu est
inactif. Les règles existantes devront être adaptées après lecture de cette
sortie, puis sauvegardées par le mécanisme de persistance déjà installé. Le
script générique refuse désormais d'activer automatiquement un UFW inactif.

Dans l'Oracle Cloud Console, appliquer les mêmes entrées à la Security List ou
au NSG du VNIC :

- TCP 22 depuis vos IP d'administration si possible ;
- TCP 80 et 443 depuis Internet ;
- TCP 21115, 21116 et 21117 depuis Internet ;
- UDP 21116 depuis Internet ;
- aucun accès public à 8443, 1080 ou aux anciens ports 31xxx.

`21118-21119/tcp` restent physiquement sur Oracle mais ne sont ouverts que si
un client Web RustDesk est réellement déployé. Le client natif Windows/Linux
n'en a pas besoin.

Docker peut contourner certaines règles UFW pour les ports explicitement
publiés par un conteneur. Le Compose RelaisDesk ne publie que 21115-21117 ; les
règles OCI/NSG restent la seconde barrière obligatoire et aucun autre
conteneur ne doit publier 8443, 1080 ou 21118-21119.

## 16. Publier le site uniquement chez OVH

Téléverser le contenu du dossier Windows `relaisdesk/` vers la racine web de
l'hébergement OVH (`www/` ou le dossier multisite configuré). Utiliser le FTP
OVH, SFTP s'il est disponible, ou le gestionnaire de fichiers OVH. Inclure les
`.htaccess` et les artefacts signés de `relaisdesk/downloads`.

Dans OVH Manager :

1. rattacher `relaisdesk.fr` et `www.relaisdesk.fr` au même dossier multisite ;
2. activer SSL pour ces deux noms ;
3. laisser OVH générer et renouveler leur certificat ;
4. ne créer aucune redirection ou proxy vers Oracle pour les pages HTML ;
5. conserver `relaist.cluster129.hosting.ovh.net` en préproduction tant que
   nécessaire, avec `noindex`.

Pendant la préproduction, garder
`PUBLIC_WEBSITE_URL=https://relaist.cluster129.hosting.ovh.net`. Ne passer à
`https://relaisdesk.fr` qu'au moment où le site définitif répond chez OVH.

## 17. Recette finale obligatoire

Depuis Windows :

```powershell
.\scripts\preproduction-check.ps1 -LiveApiUrl https://api.relaisdesk.fr
curl.exe -I https://relaisdesk.fr/
curl.exe -I https://www.relaisdesk.fr/
curl.exe -I https://api.relaisdesk.fr/api/v1/health
Test-NetConnection api.relaisdesk.fr -Port 21115
Test-NetConnection api.relaisdesk.fr -Port 21116
Test-NetConnection api.relaisdesk.fr -Port 21117
```

Effectuer ensuite une session réelle de plus de cinq minutes avec :

1. activation d'une licence technicien ;
2. génération et activation d'un code viewer ;
3. connexion P2P ;
4. connexion forcée par relais ;
5. renouvellement du jeton après cinq minutes ;
6. fermeture du launcher et coupure du canal ;
7. révocation d'un viewer ;
8. dépassement du quota de techniciens simultanés ;
9. connexion à l'espace client, facture, renouvellement et export CSV ;
10. téléchargement d'un artefact et vérification de sa signature/manifeste.

Sur Oracle :

```bash
sudo ss -lntup
sudo systemctl status relaisdesk-api relaisdesk-backup.timer --no-pager
sudo docker compose -f /srv/relaisdesk-src/rustdesk-server/docker-compose.yml ps
sudo journalctl -u relaisdesk-api --since '30 minutes ago' --no-pager
sudo certbot renew --dry-run
sudo systemctl start relaisdesk-backup.service
sudo journalctl -u relaisdesk-backup.service -n 100 --no-pager
```

La sortie de `ss` ne doit pas montrer `0.0.0.0:8443` ni `[::]:8443`.

## 18. Retour arrière

Conserver les anciennes unités arrêtées et la sauvegarde jusqu'à la fin de la
recette. En cas d'échec :

1. arrêter `relaisdesk-api`, `hbbs` et `hbbr` nouveaux ;
2. restaurer la base sauvegardée et les trois fichiers RustDesk ;
3. redémarrer les anciennes unités ou les anciens conteneurs ;
4. remettre l'ancien bloc Nginx de l'API si nécessaire ;
5. vérifier `api.relaisdesk.fr` et les trois ports RustDesk ;
6. ne pas modifier les DNS du site OVH, qui sont indépendants de ce retour.

## Références officielles pour les opérations externes

- [OVHcloud — documentation DNS et enregistrements](https://help.ovhcloud.com/csm/fr-sn-documentation-web-cloud-domains?id=kb_browse_cat&kb_category=54441955f49801102d4ca4d466a7fdb2&kb_id=e17b4f25551974502d4c6e78b7421955)
- [Certbot — utilisation de Nginx, plusieurs domaines et `--cert-name`](https://eff-certbot.readthedocs.io/en/stable/using.html)
- [Docker — installation officielle sur Ubuntu](https://docs.docker.com/engine/install/ubuntu/)
- [Oracle Cloud — Security Lists et Network Security Groups](https://docs.oracle.com/en-us/iaas/tools/oci-cli/latest/oci_cli_docs/cmdref/network/security-list.html)
- [SignPath Foundation — conditions open source](https://signpath.org/terms)
- [SignPath — intégration GitHub](https://docs.signpath.io/trusted-build-systems/github)
