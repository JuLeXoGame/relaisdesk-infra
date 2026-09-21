# Suivi juridique du 12 septembre 2026 — sans intervention API

Périmètre demandé : corriger le site et les documents, laisser l'API et le dossier
de médiation en l'état. Ce document n'est ni une attestation de conformité ni une
preuve de démarche accomplie. Aucun envoi ANSSI, accord fournisseur, publication
GitHub, déploiement Oracle ou changement des exécutables n'est réalisé ici.

## 1. Livraison locale et publication OVH

Les fichiers de référence sont directement dans `relaisdesk/`, sans archive ZIP :

- `index.html`, `i18n.js` : compatibilité Windows/Linux, macOS non disponible
  publiquement, signatures Ed25519 distinctes du chiffrement, absence de garantie
  automatique RGPD ou de souveraineté du seul fait d'un hébergement en France ;
- `client/index.html`, `client/styles.css`, `client/app.js` et le nouveau
  `client/subscription-cancellation.js` : récapitulatif et confirmation explicite
  de résiliation, même endpoint et même requête `{id}` vers l'API existante ;
- `cgv.html` : seul déplacement du paragraphe de quota pour retrouver exactement
  l'ordre de l'archive contractuelle déjà intégrée, sans changement des clauses ;
- `politique-confidentialite.html` : références des fournisseurs et moyen de
  demander les garanties ; durées et finalités inchangées ;
- `logiciel-libre.html` : distinction des lots publics et locaux, sous-modules et
  instructions de construction, limites explicites de la traçabilité.

Transférer ces fichiers dans la même arborescence sur OVH, notamment le nouveau
script client. Ne pas transférer ce document interne, l'API, les secrets ni les
programmes de test. Les CGV/DPA restent `2026-09-11` et les conditions d'essai
`2026-09-11-fleet-v2`. Aucun redémarrage ni changement d'environnement Oracle
n'est nécessaire. Les mentions existantes sur la médiation ne sont pas supprimées.

## 2. Provenance : faits établis et lacunes

Au contrôle public du 12 septembre, le manifeste propose le lot `1.0.2`, daté
`2026-09-11T10:22:39Z`. Ce n'est pas le lot local de `relaisdesk/downloads/`,
également nommé `1.0.2`, daté `2026-09-11T23:32:50Z`.

Preuve positive : le Technicien public a l'empreinte
`0479b842bed81c13e250a0b6922ba93d5e2e9b508bb9ffd82824aaba4e22da43`.
La copie conservée dans `.cache/downloads-refresh-20260911/downloads/` porte
cette empreinte et contient octet pour octet le wrapper moteur
`86a2132b04743e3c2199cc20a86e158412bcb549e4e9ab247427a9a463f326c0`.
Le compte rendu du 8 septembre rattache ce moteur au commit client
`e34c8bab932791961b2406bac5d0256b744b40b1`, sous-module protocole
`924efac1e2d1002dcad3de51938481bd29d7e46a`. Cette preuve ne vaut pas reconstruction
de l'ensemble des programmes ni identification de chaque autre paquet.

Le lot local 1.0.2 incorpore un autre moteur Windows (`b73131f3…`) : ne pas lui
attribuer le commit e34 sans preuve. Sa notice historique n'est pas une preuve
de compilation. Les nouveaux Windows 1.0.3 restent dans
`installer/build/test-securite-1.0.3/`, non publiés, avec leur
[recette et empreintes](RECETTE_SECURITE_1.0.3_WINDOWS.md).

Les sources publiques client, serveur et protocole sont accessibles sur GitHub.
Les sources Go des lanceurs ne figurent pas dans ces dépôts. Pour clôturer ce point :

1. Identifier chaque binaire par nom, SHA-256 et date du manifeste, pas seulement
   par son numéro ; conserver la source exacte utilisée, les scripts, dépendances
   et notices. Ne pas attribuer les sources actuelles à un ancien binaire.
2. Déterminer le périmètre AGPL des lanceurs et des scripts nécessaires, et choisir
   explicitement la licence du code indépendant. Ne pas déduire qu'un appel API
   oblige à publier tous les secrets ou toute l'API.
3. Préparer une sélection explicite des sources à publier, contrôler contenu et
   historique, puis publier les éléments requis. Ne jamais publier mécaniquement
   la racine : elle contient des données et secrets locaux.
4. Vérifier une reconstruction depuis les sources publiées et conserver les
   anciennes sources. Une identité binaire bit à bit n'est pas une condition
   générale de l'AGPL ; la source correspondante complète reste nécessaire.

Le changement de la page web ne règle pas, à lui seul, une livraison de sources
incomplète. Aucune nouvelle licence ni publication n'est décidée par cette note.
Référence : [AGPLv3, articles 1, 5, 6 et 13](https://www.gnu.org/licenses/agpl.en.html).

## 3. Prestataires et garanties RGPD : pièces à réunir

Conserver les pièces réelles dans un espace privé, jamais dans `relaisdesk/`.
Une URL vers les conditions publiques ne prouve pas leur acceptation pour le compte.

| Prestataire / sujet | Contrôle à documenter | Preuve actuellement vérifiée |
| --- | --- | --- |
| Oracle OCI | Entité contractante, accord Cloud/DPA applicable au compte, région Marseille, sous-traitants et pays d'accès support, mécanisme de transfert et analyse complémentaire si nécessaire | Région documentée antérieurement ; contrats du compte non contrôlés ici |
| Stripe | Pays du compte, version DPA/DTA acceptée, rôles, destinataires, transferts et mécanisme applicable | Textes publics consultés ; acceptation du compte à rapprocher |
| OVH / SMTP | Contrats des offres réellement souscrites, DPA, sous-traitants et localisation | Textes publics consultés ; contrats du compte non contrôlés ici |
| Google Drive | Ne pas traiter comme une sauvegarde active avant connexion et vérification contractuelle ; anticiper l'information des clients selon l'annexe RGPD | Non activé par cette livraison |

Pour chaque ligne, renseigner : date du contrôle, référence du contrat et de sa
version, lieu privé de conservation, finalité, données, pays, garantie applicable,
responsable, prochaine revue et éventuel écart. Ne pas cocher « validé » sur la
seule base de l'existence d'une page fournisseur. Donner suite aux demandes de
copie des garanties sans divulguer les secrets ou données d'autres clients.

Sources : [Stripe DPA](https://stripe.com/fr/legal/dpa),
[Stripe DTA](https://stripe.com/fr/legal/dta),
[contrats Oracle Cloud](https://www.oracle.com/contracts/cloud-services/),
[protection des données OVH](https://www.ovhcloud.com/fr/personal-data-protection/),
[RGPD article 13](https://www.cnil.fr/fr/reglement-europeen-protection-donnees/chapitre3).

## 4. Conservation : organiser la revue, sans toucher à la production ici

Appliquer la [procédure du parc](JURIDIQUE_PARC_ET_ACCES_PERMANENT.md), au moins
chaque semaine. Aucun automate, effacement ou nouvelle durée n'est activé.
Le contrôle doit couvrir la base active, les anciennes sauvegardes de déploiement,
les snapshots Oracle et les éventuelles copies distantes. Une sauvegarde sur le
même VPS reste une copie de données soumise à une durée et à des accès maîtrisés.

Modèle de preuve minimale à compléter dans un registre privé :

| Date / opérateur | Périmètre et échéance | Renouvellement revérifié | Restitution / effacement / exception motivée | Contrôle effectué | Copies et retraits à réappliquer | Prochaine action |
| --- | --- | --- | --- | --- | --- | --- |
| À renseigner lors de l'opération réelle | | | | | | |

Ne pas recopier tout le parc ou les secrets dans ce registre. Pour chaque jeu de
sauvegardes, documenter la durée/rotation réelle, les accès, l'emplacement et le
test de restauration. Ne supprimer aucune sauvegarde tant que son périmètre et
les exigences de conservation/restauration n'ont pas été validés. Avant remise
en service d'une restauration, réappliquer les retraits et effacements enregistrés.

## 5. ANSSI : compléter le dossier existant, sans réécrire les originaux

L'envoi initial est déclaré effectué par l'exploitant ; ni sa référence ni la
prise en compte des rectificatifs ne sont démontrées par les seuls fichiers locaux.
Les PDF signés initiaux et les quatre PDF rectificatifs du 7 septembre restent
intacts. Ne pas renvoyer une ancienne pièce inexacte comme document à jour.

Reprendre le courriel du dossier existant et vérifier, avant envoi :

- si les quatre rectificatifs du 7 septembre ont déjà été transmis et reçus ;
- les versions distinctes du client, des lanceurs et du serveur, leurs commits
  et les empreintes des programmes réellement distribués ;
- la description du chiffrement des sessions (ne pas confondre XSalsa20-Poly1305,
  Ed25519 et TLS), de l'enrôlement permanent et des preuves de possession ;
- le stockage local des secrets et le périmètre exact des nouveaux Windows 1.0.3,
  encore en test, sans les présenter comme déjà distribués ;
- la brochure commerciale (Pro 110 €, parc 500/1 000/2 000 à 4 500 postes,
  essai de 30 jours, compatibilités réelles) et le manuel d'accès permanent ;
- le justificatif d'immatriculation attendu pour l'EI.

Demander au bureau instructeur si un formulaire complet ou une nouvelle démarche
est nécessaire compte tenu des changements. Ne pas déduire une autorisation
d'exporter ou un classement « grand public » de la seule publication AGPL.
Les changements de prix seuls ne prouvent pas une modification cryptologique.
Le [dossier initial](DOSSIER_DECLARATION_ANSSI.md) et les
[rectificatifs historiques](RECTIFICATIFS_JURIDIQUES_2026-09-07.md) restent référencés.

## 6. Autres preuves externes

- CRA : appliquer la [procédure d'incident actualisée](PROCEDURE_INCIDENTS_CRA_RGPD.md),
  vérifier le périmètre fabricant et réaliser un exercice interne. L'échéance du
  11 septembre 2026 est passée ; aucune fausse notification ne doit être envoyée.
- RNE / activité / assurance : conserver le justificatif actuel, vérifier que
  l'activité de service et la couverture d'assurance correspondent à RelaisDesk.
  Cette note n'ajoute pas une activité et ne crée pas une seconde EI.
- Facturation : vérifier le canal de réception électronique applicable depuis
  septembre 2026 ; l'émission pour une micro-entreprise relève du calendrier 2027.
  Aucun changement du système de facturation/API n'est effectué.

## 7. Contrôles locaux

Exécuter `node scripts/legal-checks.mjs`, `node scripts/fleet-legal-checks.mjs`,
`node scripts/legal-web-20260912-checks.mjs` et
`python -B scripts/legal_contract_export.py`. Le dernier contrôle lit la version
du mailer et vérifie l'égalité intégrale HTML/DPA/archive ; il ne réécrit rien.
Les tests d'interface utilisent des données fictives et n'envoient aucune demande
de résiliation à l'API réelle.

Résultat du contrôle local : ces quatre vérifications passent. Un essai Chromium
hors réseau, aux largeurs de 1 280, 375 et 320 pixels, vérifie également que le
récapitulatif est visible à l'ouverture, que la touche Échap abandonne la demande
et que la confirmation fonctionne. Les 167 fichiers surveillés dans l'API, les
fichiers de base de données et les lots de programmes n'ont pas changé d'empreinte.
Cette recette locale ne remplace pas le contrôle après transfert sur OVH.
