# Architecture

RelaisDesk ajoute une couche commerciale et un contrôle d'accès réseau à
RustDesk Community au moyen de forks AGPL maintenus dans `rustdesk/` et
`rustdesk-server/`.

## Flux

1. Le technicien installe RelaisDesk.
2. L'application valide `license_id` + `license_key`, ou le code viewer, via
   l'API HTTPS.
3. Elle crée une clé Ed25519 locale propre au rôle et obtient un jeton signé de
   cinq minutes, lié à la clé publique de cet appareil.
4. Le configurateur écrit uniquement les chemins des deux fichiers secrets dans
   `RustDesk2.toml`; le jeton est renouvelé à chaud avant son expiration.
5. Le client signe ses enregistrements et demandes de connexion. `hbbs` vérifie
   signature, expiration, anti-rejeu, rôle, locataire et quota avant de mettre
   les deux postes en relation.
6. RustDesk contacte directement `hbbs` sur `21116/tcp+udp`. La traversée NAT
   utilise également `21115/tcp`.
7. Si une connexion directe échoue, le fork utilise `hbbr` sur `21117/tcp`.
   `hbbr` exige deux autorisations du même locataire et les rôles complémentaires
   technicien/viewer.
8. Chaque extrémité contrôle son canal signé toutes les dix secondes. Si l'API
   ne renouvelle plus le jeton technicien ou viewer (révocation, expiration,
   fermeture du launcher), l'extrémité concernée ferme la session.

Aucun proxy applicatif n'intervient. Le flux d'écran reste P2P quand le NAT le
permet et conserve le chiffrement natif de bout en bout de RustDesk.

## Serveur

- API : `127.0.0.1:8443`, publiée par le Nginx Oracle sur
  `https://api.relaisdesk.fr`. Le site `relaisdesk.fr` reste hébergé chez OVH.
- Fork RustDesk Community RelaisDesk : `hbbs` sur `21115-21116/tcp` et
  `21116/udp`, `hbbr` sur `21117/tcp`.
- DB : `/data/relaisdesk/licences.db`.

## Donnees

La table canonique est `licences` avec `license_id`, `license_key`,
`status`, `expires_at`, `max_connections` et `current_connections`.

Le locataire commercial est identifié durablement par `customers.id` et par
un identifiant public `CUS-…`. Les adresses autorisées sont des membres de
`customer_users`; l'adresse de facturation n'est donc plus une clé primaire du
locataire. Licences, commandes, factures et sessions pointent vers
`customer_id`. Un changement d'adresse ou l'ajout d'un membre ne sépare plus
l'historique commercial du client.

`max_connections` est encodé dans un jeton court par l'API, puis vérifié par
`hbbs` comme nombre maximal de clés d'appareil technicien actives. Les codes
viewer restent rattachés au `technician_license_id`; ce lien devient le
`tenant` signé et empêche les connexions entre clients de comptes différents.
Au premier jeton réseau, le code viewer est en outre lié durablement à la clé
Ed25519 de cet appareil. Le même appareil peut renouveler son jeton, mais une
copie du code ne peut plus être réutilisée depuis une autre machine. L'annonce
de l'identifiant RustDesk exige également cette clé liée, afin qu'un tiers ne
puisse pas remplacer l'identifiant présenté au technicien.

Le champ SQLite historique `current_connections` n'est pas la source de vérité
du quota : hbbs conserve les présences actives en mémoire avec une expiration
de 35 secondes. L'API marque donc ce compteur comme indisponible pour la
télémétrie temps réel au lieu d'afficher un faux zéro. Une installation inactive
n'est jamais enregistrée dans ce compteur hbbs et ne consomme aucun créneau.

## Espace client commercial

L'espace `relaisdesk/client/` est séparé de l'espace technicien. Une clé de
licence ne permet jamais d'y accéder : le titulaire reçoit par e-mail un lien
à usage unique valable quinze minutes, échangé contre une session opaque dont
seul le hachage est conservé. La session expire après huit heures au plus ou
trente minutes d'inactivité et le navigateur la garde uniquement dans
`sessionStorage`.

Après vérification du lien, l'adresse est résolue vers son appartenance dans
`customer_users`. Toutes les requêtes commerciales sont ensuite filtrées par
le `customer_id` stable : licences sans leur clé secrète, commandes, factures
protégées et interventions. Les réponses authentifiées portent
`Cache-Control: no-store`.

## File de traitements persistante

Stripe, les liens de connexion et les courriels commerciaux passent par la
table SQLite `jobs`. Le webhook Stripe authentifie et enregistre l'événement
avant de répondre, puis un worker intégré unique traite au plus vingt travaux
par passe. Les clés d'idempotence empêchent les doubles traitements, les
verrous expirent après quinze minutes et les échecs sont repris avec un délai
exponentiel. Les travaux terminés ou définitivement échoués sont purgés après
trente jours.

Cette file réutilise la base et le processus API : elle apporte la persistance
sans courtier externe ni collecte de métriques. Le jeton d'un lien magique est
créé seulement au moment de l'envoi et n'est jamais conservé en clair dans le
contenu du travail. Une configuration SMTP absente n'est jamais considérée
comme une livraison réussie : le travail reste en échec et suit sa politique de
reprise.

## Sauvegardes

La commande `relaisdesk-backup` produit un instantané transactionnel avec
`VACUUM INTO`, vérifie SQLite, chiffre par blocs authentifiés AES-256-GCM puis
effectue immédiatement un déchiffrement et un second contrôle d'intégrité. Un
timer systemd l'exécute quotidiennement. La copie facultative hors machine est
indépendante de la rétention locale.

## Renouvellements et relances

Un renouvellement crée une commande de type `renewal` liée à la licence
existante. Le webhook Stripe signé, ou la validation administrative d'un
virement, prolonge atomiquement cette licence de trente jours à partir de son
échéance si elle est encore active, ou de la date du paiement si elle a expiré.
Les identifiants de licence ne changent pas et un double webhook ne peut pas
prolonger deux fois la même commande.

Le traitement horaire envoie les rappels aux paliers 14, 7, 3 et 1 jour, puis
après expiration. Chaque palier et chaque échéance sont réclamés en base avant
l'envoi pour éviter les doublons entre redémarrages ou réplicas. Le client peut
désactiver ces messages. Il n'existe aucun prélèvement récurrent ni reconduction
tacite dans ce flux : le paiement reste une action expresse.

## Historique professionnel

La génération d'un code Viewer crée une fiche `interventions`. L'activation du
Viewer la marque « client prêt » ; le technicien démarre et clôture ensuite la
fiche afin d'enregistrer une durée réelle et son compte rendu. Le titulaire
peut consulter et exporter l'historique en CSV depuis l'espace commercial.
L'export neutralise les formules de tableur. Aucun écran, fichier, mot de passe,
contenu de session ou adresse IP n'est copié dans cette table.
