# Journal des modifications (CHANGELOG) — RelaisDesk

## [CGV : paiement en crypto-actifs] - 2026-09-21

- CGV `2026-09-21` et conditions d'essai `2026-09-21-fleet-v2` : ajout du
  paiement en Bitcoin/XRP pour les achats prépayés (cours garanti affiché,
  frais réseau à la charge du client, remboursement en euros par virement).
- Politique de confidentialité mise à jour (données de paiement crypto,
  registres publics, conservation dix ans avec la facture).
- Constantes API, pièces jointes e-mail et scripts de contrôle alignés ;
  archives du 11 septembre conservées à l'identique et épinglées par SHA256.
- Cahier technique du module d'encaissement (BTCPay + XRP sur Oracle) dans
  `docs/CAHIER_TECHNIQUE_PAIEMENT_CRYPTO_2026-09-21.md`.

## [Viewer : réenrôlement automatique] - 2026-09-21

- Saisir un nouveau code permanent remplace désormais l'accès permanent
  existant sans passer par `--unenroll` : le Viewer Windows retire
  l'ancien enrôlement puis recommence, comme la CLI et le GUI Linux le
  faisaient déjà (prédicat partagé `isFleetReplaceableError`).

## [Moteur avec suivi d'intervention] - 2026-09-21

- Nouveau moteur Windows compilé depuis le fork avec le suivi
  d'intervention (`connected`/`closed` vers le pont local) : les fiches se
  clôturent désormais automatiquement à la fin de la prise en main.
- Nouveaux blobs embarqués et empreintes épinglées (portable et natif) ;
  le script de build injecte l'identité native aux tests du Viewer.

## [Espace unifié : actions d'intervention] - 2026-09-20

- Ajout des actions Démarrer / Clôturer / Annuler sur les fiches
  d'intervention de l'espace client unifié (la page technicien autonome
  redirige vers cet espace) : colonne Actions, dialogue de clôture avec
  compte rendu, libellés traduits (fr, en, de, es, it, ru, pl).
- Nouvel endpoint `POST /api/v1/customer/interventions/{id}/{start,complete,cancel}`,
  limité aux licences du client connecté ; mêmes règles d'état que côté
  technicien (durée minimale d'une minute, clôture idempotente).
- Le pont de suivi du configurateur solde la fiche à son arrêt (clôture si
  la session était établie, annulation sinon) au lieu de la laisser ouverte
  quand l'événement final du moteur n'arrive pas ; journal par session dans
  `authorization/interventions/<id>.log`.
- Le configurateur redémarre le moteur RustDesk après l'écriture de sa
  configuration : le moteur ne relisant ses options qu'au démarrage, une
  instance déjà en cours ignorait sinon le suivi d'intervention.

## [Fiabilité commerciale] - 2026-08-26

- Remplacement de l'adresse e-mail comme clé de locataire par un
  `customer_id` stable et une table de membres autorisés ; migration automatique
  des licences, commandes, factures et sessions existantes.
- Ajout d'une file SQLite persistante et idempotente pour les webhooks Stripe,
  liens de connexion, livraisons, factures, virements, rétractations et relances,
  avec reprise après redémarrage et délai exponentiel.
- Ajout de sauvegardes quotidiennes SQLite cohérentes, vérifiées, chiffrées en
  AES-256-GCM, testées à blanc et copiables vers un volume hors machine.
- Ajout du contrôle de cohérence SMTP, d'un outil ponctuel SPF/DKIM/DMARC et
  d'en-têtes transactionnels standards (`Date`, `Message-ID`, `Reply-To`,
  `Auto-Submitted`).
- Une configuration SMTP absente ne simule plus une livraison réussie ; le
  travail reste désormais réessayable. Correction de l'écran de connexion qui
  pouvait rester visible après l'ouverture de l'espace client.
- Aucun service de supervision, courtier externe ou collecte de métriques n'a
  été ajouté à l'API.

## [Espace client commercial] - 2026-08-25

- Ajout d'un espace client sans mot de passe, protégé par lien à usage unique,
  sessions opaques limitées dans le temps et séparation stricte des clients.
- Consultation des licences sans exposer les clés, des commandes, des factures
  et téléchargement authentifié des justificatifs.
- Renouvellement explicite par Stripe ou virement : après confirmation du
  paiement, la licence existante est prolongée de 30 jours sans changer son
  identifiant, sa clé ni ses métadonnées internes.
- Relances automatiques et dédupliquées avant/après échéance, avec préférence
  de désactivation dans l'espace client.
- Historique professionnel des interventions partagé entre espace client et
  espace technicien, cycle de vie, durée, compte rendu et export CSV protégé.
- Ajout des migrations SQLite, tests d'isolation multi-client et règles de
  rétention associées.

## [Architecture Community sans proxy] - 2026-08-23

- Retrait du proxy SOCKS5 des générateurs RustDesk Windows et Linux.
- Retrait de `proxy_port` du contrat API et de la configuration serveur.
- `RustDesk2.toml` utilise directement `hbbs`/`hbbr` et ne définit plus
  `api-server` (fonction RustDesk Server Pro) ni `disable-udp`.
- Le pare-feu ferme l'ancien port 1080 et ouvre `21115/tcp`, `21116/tcp+udp`
  et `21117/tcp`.
- Limite connue : l'expiration d'un code viewer reste un contrôle applicatif et
  ne révoque pas une configuration RustDesk déjà distribuée.

## [1.4.0] - 2026-08-20

### Création du Site Web Statique `relaisdesk.fr` & Intégration Commandes / Paiement
- **Site Web Statique (`relaisdesk/`)** :
  - `relaisdesk/index.html` : Landing page complète avec présentation de l'infrastructure, grille tarifaire, calculateur dynamique Ultra (10 à 200 techniciens), modal de souscription, espace téléchargements, vérificateur de licence public et FAQ.
  - `relaisdesk/mentions-legales.html` : Mentions légales officielles conformes LCEN (éditeur, hébergement OVH et Oracle Cloud, conformité licence AGPLv3 RustDesk).
  - `relaisdesk/politique-confidentialite.html` : Politique de protection des données conforme RGPD (minimisation, chiffrement de bout en bout des flux, absence de traceurs tiers).
  - `relaisdesk/cgv.html` : Conditions Générales de Vente et d'Utilisation professionnelles (prix, modalités Stripe/virement, renonciation rétractation contenu numérique, SLA).
  - `relaisdesk/styles.css` : Feuille de style moderne, responsive (mobile/desktop), thème sombre avec effets de cartes et animations.
  - `relaisdesk/app.js` : Logique client avec constante d'API unique (`API_BASE_URL`), gestion du calculateur Ultra, commandes Stripe / virement et vérification de licence.
  - Note : Option PayPal mentionnée pour la Phase 2.
- **Base de Données (`database/`)** :
  - Ajout de la table `orders` (`order_id`, `email`, `plan`, `technicians`, `price`, `payment_method`, `status`, `stripe_session_id`, `license_id`, `expires_at`).
  - Recalcul obligatoire et infalsifiable des prix côté serveur (`CalculateServerPrice`).
  - Fonction `CancelExpiredOrders` pour l'annulation automatique des commandes expirées (48h).
- **API Go (`api/`)** :
  - `POST /api/v1/public/order` : Création de commande avec redirection Stripe Checkout ou génération des coordonnées bancaires.
  - `POST /api/v1/stripe/webhook` : Traitement sécurisé des paiements Stripe (vérification signature HMAC, réconciliation via `client_reference_id`, génération automatique de licence 30 jours et envoi d'email).
  - `POST /api/v1/admin/orders/{order_id}/mark-paid` : Validation manuelle des virements bancaires par l'administrateur.
  - `POST /api/v1/public/license/status` : Vérification publique du statut et de l'expiration d'une licence.
  - `GET /api/v1/downloads/{binary}` : Distribution directe des binaires lourds (.exe et .deb) depuis le serveur Oracle Cloud pour contourner les quotas d'hébergement OVH.
  - `api/mailer` : Module d'expédition d'emails SMTP (livraison de licence et instructions de virement).
  - Tâche de fond "Reaper" exécutée toutes les heures dans `api/main.go` pour purger les commandes expirées.
  - CORS mis à jour pour autoriser `relaisdesk.fr`, `www.relaisdesk.fr`, `informatiqueadomicile03.fr` et `api.informatiqueadomicile03.fr`.

---

## [1.3.0] - 2026-08-20

### Adaptation aux nouvelles offres et grille tarifaire
- **Plan Starter** : 10 € / mois, 1 technicien (1 connexion simultanée), viewers illimités.
- **Plan Pro** : 20 € / mois, 1 à 10 techniciens (10 connexions simultanées), viewers illimités.
- **Plan Ultra (Sur mesure)** : Démarre à 20,00 € / mois pour 10 techniciens, +10 € par tranche de 10 techniciens supplémentaires jusqu'à 100 techniciens (ex: 100 techs = 110,00 €), puis +1 € par tranche de 10 techniciens au-delà de 100 techniciens (ex: 110 techs = 111,00 €, 200 techs = 120,00 €). Viewers illimités.
- **`keygen/plans.go`** : Module de calcul tarifaire et générateur de plans (`CalculatePlan`, `CalculateUltraPrice`, `PrintPricingGrid`).
- **`keygen/main.go`** : 
  - Ajout des options `--plan` (starter, pro, ultra) et `--technicians` dans `keygen generate`.
  - Ajout de la commande CLI `keygen pricing` pour afficher et calculer les devis.
  - Durée par défaut ajustée à 30 jours (cycle mensuel).
- **`api/handlers/admin.go`** : Support des champs `plan` et `technicians` dans `AdminCreateLicenseHandler`.
- **Documentation** : `Contexte.md`, `Logiciel technicien.md`, `ClientViewer.md`, `docs/ADMIN.md`, `keygen/README.md` mis à jour avec la nouvelle grille tarifaire.

---

## [1.2.0] - 2026-08-18

### Migration de domaine : relaisdesk.duckdns.org → informatiqueadomicile03.fr
- **`installer/configurator/config.go`** : `APIURL` → `https://api.informatiqueadomicile03.fr`, `PROXY_HOST` → `informatiqueadomicile03.fr`.
- **`installer/viewer/config.go`** : `APIURL` → `https://api.informatiqueadomicile03.fr`.
- **`installer/build.ps1`**, **`installer/configurator/build_linux.sh`**, **`installer/viewer/build_linux.sh`** : URL par défaut migrée.
- **`scripts/certbot-deploy.sh`** : Chemin Let's Encrypt → `/etc/letsencrypt/live/informatiqueadomicile03.fr/`.
- **`proxy/.env.example`** : `ALLOWED_DESTINATION_HOSTS` mis à jour avec `informatiqueadomicile03.fr`.
- **`installer/viewer/gui_linux.go`** : Correction du bug d'appel `configureRustDesk` (argument `viewerCode` manquant).
- **Documentation** : `docs/ARCHITECTURE.md`, `docs/DEPLOYMENT.md`, `docs/SECURITY.md`, `Programmes.md`, `README.md`, `installer/README.md` mis à jour avec le nouveau domaine.
- L'ancien domaine DuckDNS est conservé en commentaire dans les scripts de build pour rollback éventuel.

---

## [1.1.0] - 2026-08-17

### 1. Correction du parsing des dates SQLite & Authentification (Hotfix 401)
- **`database/database.go`** : Implémentation des fonctions tolérantes `ParseSQLiteTime` et `ParseSQLiteTimeString` prenant en charge tous les formats de date (RFC3339Nano, RFC3339, ISO8601 avec séparateur espace/T, fuseaux horaires, date seule).
- **`database/sessions.go`** & **`database/viewer.go`** : Sécurisation de tous les scans SQL de dates (`created_at`, `expires_at`, `used_at`, `last_connection_at`, `resolved_at`) avec `ParseSQLiteTime`.
- **`api/handlers/technician.go`** : Ajout de logs de diagnostic `[TechnicianLogin] DEBUG LOGIN ...` pour tracer les connexions réussies et les causes d'échec.

### 2. Standardisation des URLs API & Domaine DuckDNS
- **`installer/configurator/config.go`** : Valeur par défaut de `APIURL` mise à jour vers `https://relaisdesk.duckdns.org:8443` et `PROXY_HOST` vers `relaisdesk.duckdns.org`.
- **`installer/viewer/config.go`** : `APIURL` uniformisé comme URL de base (`https://relaisdesk.duckdns.org:8443`) sans sous-route intégrée.
- **`installer/viewer/api.go`** : Concaténation explicite de la sous-route `/api/v1/viewer/activate` lors des requêtes HTTP.
- **`installer/build.ps1`** : Mise à jour de l'URL par défaut (`https://relaisdesk.duckdns.org:8443`) et ajout de la compilation automatique du `viewer.exe` vers `installer/build/` et `installer/viewer/`.
- **`installer/configurator/build_linux.sh`** & **`installer/viewer/build_linux.sh`** : Mise à jour de l'URL par défaut vers `https://relaisdesk.duckdns.org:8443`.
- **`scripts/certbot-deploy.sh`** : Chemin Let's Encrypt mis à jour vers `/etc/letsencrypt/live/relaisdesk.duckdns.org/`.

### 3. Correction & Nettoyage de la configuration Proxy & Build
- **`proxy/.env.example`** :
  - Correction de `RUSTDESK_RENDEZVOUS_PORT=21116` (valeur officielle hbbs, ancien résidu 31115 corrigé).
  - Suppression de la variable inutile `API_URL` (le proxy interroge SQLite en direct).
  - Ajout de `relaisdesk.duckdns.org` dans `ALLOWED_DESTINATION_HOSTS`.
- **`Makefile`** : Correction du nom du binaire proxy cible `relaisdesk-proxy` (au lieu de `monproduit-proxy`).

### 4. Harmonisation de la documentation
- **`Contexte.md`** : Mise à jour des sections 4.6 et 11.7 pour acter que les clients viewers passent obligatoirement par le proxy SOCKS5 (`username = code_viewer`, `password = code_viewer`).
- **`docs/ARCHITECTURE.md`**, **`docs/SECURITY.md`**, **`docs/DEPLOYMENT.md`**, **`Programmes.md`**, **`README.md`** :
  - Clarification de la fermeture des ports RustDesk directs (`21115-21119`) sur les pare-feux publics.
  - Confirmation des seuls ports ouverts au public : `8443` (API HTTPS), `1080` (Proxy SOCKS5), `22` (SSH), `80/443` (HTTP/HTTPS Let's Encrypt).
