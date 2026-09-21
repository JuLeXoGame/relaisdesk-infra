# Déploiement de sécurité — 19 septembre 2026

## Périmètre autorisé

API et hbbs/hbbr en production. Les programmes Windows/Linux sont préparés
séparément pour les tests de l'exploitant : aucun remplacement des téléchargements
publics et aucun renouvellement du manifeste de versions. Site principal chez
OVH, VPN WireGuard et mises à jour du système inchangés.

## API — déployée et vérifiée

- Binaire actif SHA-256 : `a9571d8e3d3de45dbc2aa15ba3953e9329c3423b6303200a5a57d07e459d8036`.
- Ancien binaire : `c48e24e3fa40f845916063e4dd79e7b92b1612751b133a79d28d1c8717d0a4d3`.
- Point de retour privé : `/opt/relaisdesk/deployments/security-20260919-QrWxzNa1`.
- Migration répétée d'abord sur une copie SQLite ; intégrité et clés étrangères
  validées. Puis sauvegarde SQLite cohérente au moment de la bascule.
- Migration `20260919-google-mfa-v1` appliquée une seule fois. Les sessions et
  appareils de confiance des comptes MFA concernés ont été invalidés pour
  empêcher la conservation d'un accès obtenu par l'ancien parcours Google.
- API saine, base connectée, aucun redémarrage en boucle observé.
- Accès anonyme : HTTP 401 sur les espaces client, technicien et
  `/api/v1/admin/licences`.
- Données métier conservées : 1 licence, 3 appareils, 1 commande, 7 comptes.
- Tarifs et modalités d'essai publics identiques avant/après ; les tests n'ont
  pas créé de commande, de paiement ou de licence.
- Les deux fichiers OVH `client/index.html` et `client/app.js` ont été vérifiés
  en ligne avec le correctif de connexion Google/MFA.

Le script API compare les empreintes des téléchargements publics, de la
configuration Nginx, de l'environnement API et de WireGuard avant/après. Aucun
de ces fichiers n'a changé pendant sa bascule. Les conteneurs RustDesk n'ont pas
été redémarrés par le déploiement de l'API.

## Serveur RustDesk — déployé et vérifié

- Image active : `relaisdesk/rustdesk-server:security-20260919-amd64`.
- Identité de l'image : `sha256:a20d77c6743f47625397e7baa8ba1009f3f4308dc738dc587d0172d807a97103`.
- `hbbs` : `48efdf06157992e7023a54d51f8db4eec0f5d28a57bd8532e56934b51fc997bb`.
- `hbbr` : `3714808cbc0fa9011bd4c66584bea7f4cc1db83b954eb9f10acd8dc5204faf7f`.
- Sauvegarde privée : `/opt/relaisdesk/deployments/server-security-20260919-tS9Dk2Hf`
  (compose, environnement, Nginx et données RustDesk avant remplacement).
- Ancienne image conservée : `relaisdesk/rustdesk-server:d998a32-security-amd64`.
- Les deux processus fonctionnent avec autorisation obligatoire, clés inchangées,
  racine en lecture seule et aucun redémarrage en boucle observé.
- Ports conservés : 21115/TCP, 21116/TCP+UDP et 21117/TCP. Les ports WebSocket
  internes ne sont pas publiés par Docker. API limitée à 127.0.0.1:8443.
- Aucun remplacement de données, de clé, de licence ou de téléchargement public.
- Nginx n'a reçu qu'une route exacte de sources sur `api.relaisdesk.fr`.
  Aucun bloc du site principal RelaisDesk n'a été créé sur Oracle.

Les contrôles préalables ont d'abord interrompu la bascule sur un repère Nginx,
puis sur l'attente de ses nouveaux workers ; ils ont ensuite déclenché un retour
à l'ancienne image à cause de l'ordre des variables d'environnement Docker.
La comparaison porte désormais sur les valeurs triées et la dernière bascule a
passé tous les contrôles. Aucun retour arrière de base n'a eu lieu.

Les sources serveur publiées correspondent à la révision
`9b784dfa50a2a031e546e75acf225e90d11b0812`, au sous-module
`90fcb583f87d6cde8fe4666c9767b9646a95a43b` et au verrouillage Rustls corrigé.
L'archive est téléchargeable sans compte à l'adresse
https://api.relaisdesk.fr/sources/rustdesk-server-security-20260919.tar.gz ;
son SHA-256 vérifié depuis l'extérieur est
`c43e26f7700120127a877c0972cd801b85e39599142421f1772942001d355f72`.
Elle ne contient pas les fichiers `.env`, bases ou clés de production.

Après cette bascule, seule la ligne `SERVER_SOURCE_URL` de l'environnement API
a également été alignée sur cette archive, puis l'API a été redémarrée et
contrôlée. Cette modification est distincte de la bascule initiale de l'API.
La copie précédente est `api.env.before-source-url` dans la sauvegarde serveur.

Le fichier local `relaisdesk/logiciel-libre.html` référence cette archive : il
reste à transférer par l'exploitant sur OVH. Aucun accès OVH n'est disponible ici.

## Programmes de test

Emplacement : `output/tests/security-20260919/`, avec guide de recette.
Version applicative injectée : 1.0.4. Pas de signature Authenticode ni de nouveau
manifeste public. Les programmes ne sont pas installés sur le PC de l'exploitant.

Moteur Windows portable : `d29d6c720ad701d637e7e824cb6ab56396d9e1fe25f5fd572ba6e4d15d68a411`.
Moteur Windows natif : `80e453318b0e9a86080b220826c7dc5c8e60c9c295924f713b8fae89a33aa2ae`.
Paquet natif Linux : `0606cfcdcc582487070ae02295cb9f926675db3ef27dd7bd70a3cfb30a4f511c`.
Bibliothèque Linux : `59fa3523f126ee19b49d2329b0f3c0b92dc9c8c17a43a4de8345ebe96cbc7d45`.
Le paquet Linux conserve l'interface Flutter et les dépendances du paquet
embarqué existant ; sa bibliothèque corrigée est reconstruite et son chemin
de chargement local `$ORIGIN` conservé. Aucune dépendance manquante détectée
dans l'environnement Linux de validation.

Les empreintes des programmes finaux sont dans leurs `SHA256SUMS.txt`.
Les quatre programmes Windows sont dans `windows-v1.0.4/` ; les deux paquets
Debian et deux exécutables Linux sont dans `linux/`. Construction Windows,
tests CI et `go vet` réussis, y compris validation du paquet natif d'accès
permanent avec l'empreinte réellement injectée. Tests du Technicien et tests
de sécurité du Viewer exécutés sous Linux avec succès. Les tests d'accès à un
compte réel sont désormais explicitement opt-in, et le test des icônes Windows
ne bloque plus la compilation Linux.

Les scripts de construction distinguent l'identité des variables du programme
(`main`) et celle des tests (`viewer` / `configurator`) pour que les tests
vérifient effectivement les empreintes injectées, pas les valeurs par défaut.
Contrôles finaux réussis : huit SHA-256, formats PE/ELF/Debian, version 1.0.4 et
empreintes moteur présentes dans les lanceurs. Le serveur est resté stable
plusieurs minutes après la bascule, API et conteneurs sans redémarrage automatique.

Le manifeste public est toujours la version 1.0.0 publiée le 17 septembre 2026
à 16:13:00 UTC : les programmes de test n'ont pas remplacé les téléchargements.

## Retour arrière de l'API

Le répertoire privé conserve `api.previous` et `licences.before.db`. Ne pas
restaurer la base automatiquement : cela effacerait les nouvelles écritures
métier. Un retour à l'ancien binaire réintroduirait la faiblesse MFA corrigée ;
il ne constitue qu'une mesure d'urgence après diagnostic.

## Recette à effectuer par l'exploitant

- Connexion Google sur un compte avec MFA : TOTP ou code de secours requis,
  sans remplacement de ce facteur par un code reçu par courriel.
- Sur deux machines de test : Viewer ponctuel, technicien, prise en main,
  reconnexion, presse-papiers et transferts de fichiers autorisés.
- Accès permanent : redémarrage de la machine, présence dans le bon dossier,
  droits du technicien et refus hors de ses dossiers.
- Linux : mise à niveau du moteur existant, permissions privées des fichiers
  de preuve et fonctionnement de la session graphique via le service.

Les tests automatisés ne remplacent pas une prise en main graphique réelle.
Les nouveaux programmes ne doivent pas être publiés avant cette recette et la
préparation des sources correspondant à leurs paquets complets.
