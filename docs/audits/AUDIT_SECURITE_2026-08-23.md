# Audit de sécurité RelaisDesk — 23 août 2026

> Rapport historique, remplacé par `AUDIT_SECURITE_2026-08-24.md` après l'audit
> complet, la reconstruction des livrables et le contrôle de la préproduction.

## Résultat

Le code RelaisDesk personnalisé a été corrigé et validé sur ses huit modules Go.
Les contrôles finaux ne trouvent aucune vulnérabilité Go atteignable. Les
interfaces JavaScript passent aussi la vérification syntaxique.

Le dépôt RustDesk tiers a été mis à jour autant que possible sans migration
d'API hasardeuse : le nombre d'avis RustSec est passé de 24 à 4. Ces quatre
avis résiduels concernent uniquement les arbres Linux/macOS actuels.

## Correctifs appliqués

- Authentification : sessions administrateur courtes, révocables et stockées
  sous forme de hash ; sessions technicien hashées avec migration des anciennes
  valeurs ; suppression de l'accès direct par secret statique ou clé maître.
- Configuration : refus de démarrer avec le jeton administrateur par défaut ou
  trop court ; CORS et en-têtes proxy resserrés ; limites de débit isolées par
  route ; limites de corps adaptées aux uploads.
- Paiements : signature Stripe obligatoire, tolérance temporelle bornée,
  contrôle du statut payé et correspondance exacte commande/session/moyen de
  paiement ; création licence + paiement atomiques en transaction.
- SMTP : TLS 1.2 minimum, STARTTLS obligatoire sur 587, TLS implicite sur 465,
  délais réseau et validation des adresses/en-têtes.
- Factures : chemins confinés, numéros validés, uploads bornés et non
  écrasants, contrôle de signature PDF, HTML manuel téléchargé sous CSP sandbox,
  nettoyage en cas d'échec base de données.
- Installateurs : configuration directe RustDesk Community générée pour les
  techniciens et viewers, sans proxy ni secret de licence dans RustDesk2.toml ;
  fichiers de configuration écrits en mode 0600 et téléchargements temporaires
  non prédictibles et bornés.
- Viewer : codes vérifiés avant annonce, identifiants RustDesk validés, liaison
  non réaffectable et génération aléatoire sans biais modulo.
- Frontend : clé de licence retirée du stockage navigateur, anciennes clés
  locales nettoyées, contenu client échappé et déconnexion administrateur
  révocable.
- Distribution : une URL de téléchargement ne peut plus servir par erreur le
  binaire d'un autre produit.
- Exploitation : permissions base/configuration durcies et service systemd
  restreint (`NoNewPrivileges`, `ProtectSystem`, espaces de noms, familles
  réseau et chemins d'écriture).
- Dépendances : Go 1.26.6 minimum ; `x/image` 0.43.0 et `x/net` 0.55.0 ; mises à
  jour Rust ciblées de `bytes`, `crossbeam`, `openssl`, `quinn`, `rustls-webpki`,
  `time`, `tracing-subscriber`, `url`, `plist`, Wayland et notifications WinRT.
- Secrets : valeurs de production supprimées du code, des exemples et tests ;
  fichiers secrets, bases et artefacts ajoutés à `.gitignore` ; exemples neutres
  ajoutés.

## Vérifications exécutées

- `go test -count=1 ./...` : huit modules réussis.
- `go vet ./...` : huit modules réussis.
- Compilation croisée Linux (`GOOS=linux`, `GOARCH=amd64`, `CGO_ENABLED=0`) :
  API, base, keygen, configurateur, viewer et debpack réussis.
- `node --check` : frontends public, technicien et administrateur réussis.
- `govulncheck ./...` : aucune vulnérabilité atteignable dans les huit modules.
- `cargo metadata --locked` : verrou RustDesk cohérent.
- `tauri-winrt-notification 0.8.1` : compilation réussie.
- Compilation RustDesk complète : progression correcte jusqu'aux dépendances
  natives, puis arrêt faute de `VCPKG_ROOT` et de `libclang` sur cette machine.

## Actions obligatoires avant production

1. Révoquer et remplacer la licence administrateur et le jeton DNS présents
   dans les fichiers locaux historiques. La simple présence dans `.gitignore`
   ne révoque pas un secret déjà exposé.
2. Générer un `ADMIN_TOKEN` aléatoire d'au moins 32 caractères et configurer un
   vrai secret de webhook Stripe. Le serveur refuse désormais les valeurs
   dangereuses ou absentes selon le contexte.
3. Reconstruire et redéployer tous les exécutables. Les binaires existants dans
   `bin/`, `installer/build/` et `relaisdesk/downloads/` sont antérieurs aux
   correctifs et certains peuvent encore contenir l'ancienne clé intégrée.
4. Ne pas redéployer les anciens binaires de proxy déplacés dans
   `obsolete-proxy-artifacts/`. Le proxy SOCKS5 a été retiré de l'architecture ;
   fermer `1080/tcp` et exposer les ports RustDesk Community requis.
5. Pour distribuer RustDesk sous Linux/macOS, planifier le remplacement des
   dépendances amont responsables des quatre avis restants :
   `libgit2-sys 0.14.2` via `keepawake/shadow-rs`, `time 0.1.45` via
   `fruitbasket`, et `users 0.10/0.11` (aucune version corrigée publiée pour
   l'avis RUSTSEC-2025-0040). Le verrou Windows ne conserve plus d'avis RustSec
   connu après les mises à jour effectuées.
