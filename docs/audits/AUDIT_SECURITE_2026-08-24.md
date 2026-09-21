# Audit de sécurité et de fiabilité RelaisDesk — 24 août 2026

> **Archive historique.** Ce document décrit l'état du 24 août, antérieur à
> l'activation des forks RelaisDesk avec autorisation réseau signée. Il ne doit
> plus servir de procédure de déploiement. Voir
> `AUDIT_SECURITE_2026-08-26.md` pour l'état actuel.

## Conclusion

Les composants RelaisDesk personnalisés ont été audités, corrigés, testés puis
reconstruits. Les huit modules Go passent leurs tests et `go vet`; `govulncheck`
ne trouve aucune vulnérabilité atteignable. Les portables Windows, installateurs
NSIS, paquets Debian et l'API Linux présents dans `relaisdesk/downloads/` ont été
regénérés après les derniers correctifs et leurs empreintes sont publiées dans
`SHA256SUMS.txt`.

Le dossier `rustdesk/` est le code source amont utilisé pour l'audit des
dépendances. Il n'est pas compilé par `installer/build.ps1` : les applications
RelaisDesk embarquent le binaire Windows officiel, signé, de RustDesk 1.4.9.

## Architecture RustDesk retenue

RelaisDesk utilise directement RustDesk Community (`hbbs`/`hbbr`). Aucun proxy
SOCKS5, aucun port 1080 et aucune fonction RustDesk Server Pro ne font partie de
l'architecture active.

Le fichier généré est :

```toml
rendezvous_server = 'api.informatiqueadomicile03.fr:21116'
nat_type = 1
serial = 0

[options]
custom-rendezvous-server = 'api.informatiqueadomicile03.fr'
relay-server = 'api.informatiqueadomicile03.fr'
key = '{public_key}'
```

`api-server` est volontairement absent : la documentation RustDesk le réserve à
la connexion au compte et à la console Web Pro. La clé ci-dessus est la clé
publique du serveur RustDesk, pas une clé de licence secrète.

## Principaux correctifs appliqués

### Lanceurs et chaîne de distribution

- RustDesk figé sur la version officielle 1.4.9 pour Windows et Debian, avec
  contrôle SHA-256 avant intégration, téléchargement ou exécution.
- Suppression de l'exécution non vérifiée d'un `rustdesk.exe` trouvé dans le
  `PATH` ou dans un dossier utilisateur alors que NSIS s'exécute en administrateur.
- Vérification du binaire embarqué au démarrage et extraction atomique ; un
  fichier de même taille ne suffit plus à être considéré comme fiable.
- Validation stricte du nom d'hôte, des ports, de la clé publique et de
  l'expiration avant de tuer RustDesk ou d'écrire le TOML.
- L'API est désormais la source de vérité du serveur rendez-vous, avec
  `api.informatiqueadomicile03.fr` par défaut.
- Les raccourcis pointent vers les wrappers RelaisDesk, plus directement vers
  RustDesk. Le Viewer supprime sa configuration temporaire après lancement et
  annonce l'ID avant de se fermer.
- Génération atomique des paquets Debian et propagation correcte des erreurs
  d'écriture.
- Génération automatique de `SHA256SUMS.txt` à chaque build.

### API, authentification et données

- Refus de démarrer avec le jeton administrateur par défaut ou trop court,
  avec une configuration serveur/ports invalide, sans TLS de production ou sans
  SMTP de production valide.
- Sessions administrateur et technicien aléatoires, expirables, révocables et
  stockées sous forme de hash ; le secret maître n'est accepté qu'au login.
- Limites de débit indépendantes pour chaque route sensible, corps HTTP bornés,
  CORS limité à des origines exactes et confiance des en-têtes proxy restreinte.
- Webhook Stripe signé et borné dans le temps ; cohérence commande/session et
  création de licence atomique.
- Coordonnées bancaires refusées si l'IBAN, le BIC ou le titulaire sont factices
  ou invalides ; aucune commande par virement n'est alors créée.
- SMTP limité aux ports 465/587, TLS 1.2 minimum et adresses/en-têtes contrôlés.
- La santé renvoie `503` si SQLite ne répond pas et n'expose plus de statistiques.
- Numéros de facture uniques même avec des trous et créations concurrentes ;
  uploads bornés, non écrasants, chemins confinés et signature PDF vérifiée.
- Exports de clés créés en permissions privées (`0600`).

### Interfaces et site

- Échappement des données API avant toute insertion HTML et suppression des
  gestionnaires HTML `onclick`.
- CSP stricte pour les espaces Admin/Technicien et CSP publique autorisant
  uniquement le script local et le hash exact du JSON-LD.
- Toutes les mutations d'interface Fyne issues des goroutines réseau repassent
  sur le thread UI, afin d'éviter courses et plantages.
- Le tableau de bord natif reconnaît le vrai moyen de paiement `bank_transfer`
  et recharge correctement la liste après validation.
- Les zones Admin/Technicien et la préproduction sont marquées `noindex` ; les
  index de répertoires sont désactivés.

### Déploiement

- `/opt/relaisdesk` reste propriété de `root`; seuls les répertoires de données
  et journaux sont modifiables par le service.
- Le hook Certbot utilise le certificat exact de
  `api.informatiqueadomicile03.fr`, vérifie sa présence et conserve la clé privée
  `root:relaisdesk` en mode `0600`.
- Le pare-feu ouvre 80/443 et les ports RustDesk Community nécessaires, ferme le
  port interne 8443 ainsi que les anciens ports proxy/commerciaux.
- La documentation TLS fondée sur l'ancienne adresse IP a été remplacée.

## Vérifications finales

- `go test -count=1 ./...` : réussite des 8 modules.
- `go vet ./...` : réussite des 8 modules.
- `govulncheck ./...` : 0 vulnérabilité appelée dans les 8 modules. Les trois
  avis présents dans l'arbre Fyne ne sont pas atteignables par le code.
- `node --check` : scripts public, Admin, Technicien et service workers valides.
- JSON des trois manifestes, XML du sitemap et JSON-LD : valides.
- Liens/assets statiques locaux : aucune cible manquante détectée.
- Scripts Bash de déploiement : `bash -n` réussi.
- Build complet : Windows, Linux, Debian et NSIS réussi.
- Contrôle des empreintes finales : toutes les lignes de `SHA256SUMS.txt`
  correspondent aux fichiers distribués.
- RustDesk officiel embarqué : SHA-256
  `eaedeb0088e687bf46f7c46a9c6ea5493ce51f3134dfd6acbedb47b5b9136274`
  et signature Authenticode valide.

## Dépendances RustDesk

Le verrou RustDesk a reçu des mises à jour ciblées, dont `anyhow 1.0.104`,
`memmap2 0.9.11` et `event-listener 5.4.2`. `cargo metadata --locked` réussit.
La compilation complète du source local reste bloquée sur cette machine par
l'absence de `VCPKG_ROOT` requise par `magnum-opus`.

`cargo audit` signale encore quatre vulnérabilités dans le verrou
multi-plateforme : `libgit2-sys 0.14.2`, `time 0.1.45` et `users 0.10/0.11`.
L'analyse `cargo tree --target x86_64-pc-windows-msvc` confirme qu'elles ne font
pas partie de l'arbre Windows. Elles devront être résolues avant toute
distribution d'un RustDesk recompilé localement pour Linux/macOS. `users` ne
dispose actuellement d'aucune version corrigée pour RUSTSEC-2025-0040.

## Actions impératives avant production

1. Révoquer et remplacer la licence administrateur et le jeton DNS encore
   présents dans les deux fichiers locaux historiques. Ils sont ignorés par
   Git, mais cela ne révoque pas une valeur déjà exposée. Les déplacer ensuite
   hors du projet.
2. Signer les quatre exécutables RelaisDesk avec un certificat de signature de
   code. Ils sont reconstruits et hachés, mais restent `NotSigned` et peuvent
   donc déclencher SmartScreen.
3. Déployer explicitement la nouvelle version : aucune modification de ce
   dossier n'a été envoyée automatiquement chez OVH ou sur le serveur API.
4. Corriger le certificat de `relaist.cluster129.hosting.ovh.net` et déployer le
   `.htaccess` local afin que la préproduction envoie `X-Robots-Tag: noindex`.
5. Remplir `/etc/relaisdesk/api.env` avec les vrais secrets Stripe, SMTP,
   administrateur et coordonnées bancaires, puis conserver ce fichier en
   `root:relaisdesk 0640`.
6. Chiffrer les sauvegardes SQLite : les clés de licence doivent rester
   consultables par l'administration et sont donc sensibles au repos.

## Limites structurelles de RustDesk Community

La suppression locale de `RustDesk2.toml` oblige normalement à repasser par le
wrapper RelaisDesk, mais ne peut pas interrompre une instance déjà lancée ni
empêcher un administrateur du poste de copier la configuration. De même, les
quotas « techniciens simultanés » ne constituent pas un contrôle réseau dur sur
`hbbs`/`hbbr`. Une révocation centrale forte nécessiterait une fonctionnalité
serveur dédiée ; elle ne doit pas être promise avec l'architecture Community
directe actuelle.

## Références

- RustDesk 1.4.9 : https://github.com/rustdesk/rustdesk/releases/tag/1.4.9
- Configuration client RustDesk : https://rustdesk.com/docs/en/self-host/client-configuration/
- RUSTSEC-2024-0013 : https://rustsec.org/advisories/RUSTSEC-2024-0013
- RUSTSEC-2020-0071 : https://rustsec.org/advisories/RUSTSEC-2020-0071
- RUSTSEC-2025-0040 : https://rustsec.org/advisories/RUSTSEC-2025-0040
