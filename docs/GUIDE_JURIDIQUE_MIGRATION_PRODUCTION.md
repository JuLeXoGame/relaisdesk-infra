# Guide juridique RelaisDesk : migration, publication et premières ventes

Complément du 21 septembre 2026 : les CGV passent à `2026-09-21` et les conditions d'essai à `2026-09-21-fleet-v2` (ajout du paiement en crypto-actifs Bitcoin/XRP pour les achats prépayés, remboursement en euros par virement). L'annexe RGPD reste `2026-09-11`. Cette livraison modifie l'API et le web de façon coordonnée (constantes, pièces jointes e-mail, scripts de contrôle et `deploy_api_legal_update.sh` alignés) ; prix, quotas, essai de 30 jours et parcours Stripe inchangés. Cahier technique du module d'encaissement : [CAHIER_TECHNIQUE_PAIEMENT_CRYPTO_2026-09-21.md](CAHIER_TECHNIQUE_PAIEMENT_CRYPTO_2026-09-21.md).

Actualisation du 12 septembre 2026 : pour les corrections sans intervention API,
les preuves encore manquantes et les fichiers à transférer, suivre
[SUIVI_JURIDIQUE_2026-09-12.md](SUIVI_JURIDIQUE_2026-09-12.md).
Les « portes » de migration ci-dessous décrivent les prérequis d'une ouverture,
pas une nouvelle demande de déploiement. L'API reste inchangée ; CGV/DPA
`2026-09-11` et essai `2026-09-11-fleet-v2` restent les versions acceptées.

Complément du 11 septembre 2026 : les adaptations de la console de parc et de l'accès permanent, le modèle d'autorisation, les procédures RGPD et les conditions de publication sont dans [JURIDIQUE_PARC_ET_ACCES_PERMANENT.md](JURIDIQUE_PARC_ET_ACCES_PERMANENT.md). Les étapes ci-dessous restent un guide préparatoire historique, pas une attestation de démarches accomplies.

Les décisions actuelles et les pièces ANSSI à renvoyer sont détaillées dans
[RECTIFICATIFS_JURIDIQUES_2026-09-07.md](RECTIFICATIFS_JURIDIQUES_2026-09-07.md).
SignPath est une option de signature ultérieure, pas un préalable à toute
distribution. Les ventes B2C ont depuis été ouvertes par l'exploitant, mais l'absence de médiateur reste à régulariser : l'ouverture ne prouve pas la conformité. Les preuves de démarches et
contrats externes doivent toujours être fournies par l'exploitant.

Ce document organise les actions juridiques dans l'ordre d'exécution du projet.
Il complète le guide technique [`MIGRATION_OVH_ORACLE.md`](MIGRATION_OVH_ORACLE.md)
et la checklist de contrôle [`LEGAL_RELEASE_CHECKLIST.md`](LEGAL_RELEASE_CHECKLIST.md).
Il s'agit d'un guide opérationnel préparatoire, pas d'une consultation juridique
ni d'une garantie de conformité. Par prudence, les contrats et la qualification
RGPD devraient être validés par un professionnel du droit avant l'ouverture
commerciale ; cette relecture est une recommandation de maîtrise du risque et
non, à elle seule, une formalité légale obligatoire.

## Réponse courte : faut-il tout faire avant la migration ?

**Non, pas avant une migration privée et réversible de préproduction.** Les
sauvegardes, l'inventaire Oracle, l'installation de la nouvelle API, les tests
sur une copie de la base et une recette non publique peuvent commencer sans
médiateur de la consommation ni CGV définitives.

**Oui, avant l'étape concernée lorsqu'elle rend le projet public ou commercial.**
Il faut respecter les quatre portes suivantes :

| Porte | Moment exact | Ce qui doit être terminé avant de passer |
|---|---|---|
| A — publication du code | avant de publier les forks et le dépôt des launchers | licences, avis de copyright, sources correspondantes, inventaire des dépendances, absence de secrets et clarification de la marque |
| B — bascule technique publique | avant d'exposer les forks modifiés et les téléchargements au public | sources AGPL exactes, politique RGPD à jour, contrats des sous-traitants, sécurité et procédure d'incident |
| C — première vente B2B | avant d'encaisser le premier professionnel | activité déclarée, CGV B2B, factures conformes, information RGPD et annexe de sous-traitance lorsque RelaisDesk traite pour le compte du client |
| D — première vente B2C | avant d'encaisser le premier consommateur | tout le socle B2B, médiateur conventionné, parcours consommateur, rétractation, garanties légales et validation juridique |

L'ordre recommandé est donc : **préproduction privée**, puis **publication des
sources correspondantes**, puis **validation juridique B2B**, puis **bascule publique
et première vente B2B**. Les ventes B2C restent désactivées jusqu'à la porte D.

## 1. État juridique déjà préparé dans le projet

Le projet contient déjà des bases sérieuses :

- les mentions légales identifient Julien BELLOT, entrepreneur individuel, le
  SIREN/SIRET, l'adresse, le téléphone, la franchise de TVA et OVH comme
  hébergeur du site ;
- les CGV distinguent les achats prépayés (30/365 jours sans reconduction) du
  parcours d'essai (30 jours gratuits puis abonnement récurrent mensuel ou annuel) ;
- le parcours prévoit l'acceptation des CGV, la preuve de leur version et une
  demande expresse de commencement immédiat pour un consommateur ;
- un formulaire de rétractation en ligne et un accusé de réception existent ;
- la politique de confidentialité décrit OVH, Oracle, Stripe et les principales
  finalités, bases juridiques, durées et droits ;
- la page « Logiciel libre » explique l'AGPLv3, le fork et la non-affiliation à
  RustDesk ;
- l'état d'ouverture réel se vérifie dans l'API publique ; une valeur par défaut
  dans le code ne prouve pas la configuration du serveur ;
- le téléchargement public n'est pas conditionné à SignPath, mais les sources
  correspondantes et le manifeste Ed25519 restent nécessaires.

Ces documents sont des projets avancés, mais ils ne doivent pas être présentés
comme validés par un avocat.

## 2. Blocages constatés à ce jour

| Élément | État actuel | Conséquence |
|---|---|---|
| Médiateur de la consommation | absence explicitement signalée dans les CGV ; dossier laissé en l'état | obligation non régularisée ; aucune modification d'ouverture B2C dans cette livraison |
| URL des sources client et serveur | commits publics identifiés dans `relaisdesk/logiciel-libre.html` | maintenir le lien exact avec les versions effectivement distribuées |
| Dépôt des launchers/configurateurs/installateurs | la racine du projet n'est pas un dépôt Git public et ne contient pas de licence racine | candidature SignPath Foundation et publication à finaliser |
| Licence du code propre | aucune licence OSI commune n'est encore attachée au périmètre publié des launchers | choisir la licence avant publication ; cela relève aussi des conditions SignPath |
| SignPath | documentation préparée, mais acceptation et chaîne CI non démontrées | ne pas publier les exécutables comme version commerciale signée |
| Activité et nom commercial | la déclaration effective n'est pas prouvée par le dépôt | vérifier le RNE et modifier l'EI existante si nécessaire |
| RGPD B2B | annexe de sous-traitance version 2026-09-11 publiée | mettre en œuvre ses engagements et conserver les preuves |
| Prestataires internationaux | la politique indique les risques, mais les contrats, régions et garanties réels ne sont pas vérifiés | audit contractuel Oracle, Stripe, OVH et SMTP avant production |
| Versions contractuelles | CGV/DPA `2026-09-11`, essai `2026-09-11-fleet-v2` | conservées ; aucun changement d'API requis pour les corrections web du 12 septembre |
| Relecture professionnelle | non démontrée | fortement recommandée avant B2B et indispensable par prudence avant B2C |

## 3. Porte A — avant de publier les forks et les launchers

Cette porte intervient avant la section 4 du guide de migration.

### 3.1 Créer des dépôts publics propres

- Publier les forks `rustdesk`, `rustdesk-server` et `hbb_common` sans secrets,
  données clients, bases SQLite, clés privées ni anciens fichiers de production.
- Créer un dépôt public séparé pour les configurateurs, viewers, installateurs,
  scripts de construction et fichiers nécessaires à SignPath. Ne pas publier
  mécaniquement toute la racine actuelle : elle contient des fichiers locaux
  confidentiels et n'est pas un dépôt Git.
- Scanner le contenu **et l'historique Git** avant le premier push. Une simple
  suppression dans le dernier commit ne retire pas un secret de l'historique.
- Ajouter une politique de sécurité et une adresse de signalement.

### 3.2 Respecter les licences libres

- Conserver la GNU AGPLv3 et tous les avis de copyright des composants RustDesk.
- Identifier clairement les fichiers modifiés, la date, le commit exact et les
  instructions de construction.
- Publier la forme réellement modifiable du code, les sous-modules modifiés et
  les scripts nécessaires à la reconstruction.
- Pour le serveur AGPL modifié accessible par le réseau, rendre gratuitement le
  code source correspondant à la version effectivement exploitée. L'amont
  RustDesk seul ne suffit pas.
- Pour chaque binaire distribué, conserver le tag source, les dépendances, le
  manifeste, les empreintes et les avis de licences qui lui correspondent.
- Choisir explicitement une licence approuvée par l'OSI pour le code propre
  présenté à SignPath. Ne pas recopier automatiquement l'AGPL sur l'API ou les
  outils maison sans avoir décidé ce que l'entreprise souhaite ouvrir.

### 3.3 Marque et présentation

- Conserver la mention visible indiquant que RelaisDesk est indépendant de
  RustDesk et n'est pas approuvé par RustDesk.
- Utiliser « RustDesk » uniquement pour identifier l'origine ou la compatibilité,
  sans donner l'impression que RelaisDesk est l'offre officielle.
- Faire une recherche d'antériorités INPI et sur les noms de domaine avant une
  campagne publique. Le dépôt de la marque RelaisDesk est recommandé, mais
  n'empêche pas à lui seul la migration technique.

### 3.4 SignPath

SignPath est une option de signature ultérieure, pas une condition de distribution
du projet. Une candidature exige de respecter ses conditions et d'obtenir son
acceptation ; voir [`SIGNPATH.md`](SIGNPATH.md). Un exécutable peut être distribué
sans Authenticode si ses autres obligations sont respectées et son état décrit
honnêtement. Ne jamais présenter la signature Ed25519 du manifeste comme une
signature Windows ou comme l'acceptation de SignPath.

**Sortie de la porte A :** les URL publiques sont stables, les sources se
reconstruisent sur une machine vierge, les licences sont visibles, aucun secret
n'est présent et la candidature SignPath peut être déposée.

## 4. Travaux autorisés en parallèle sur la préproduction privée

Avant les portes B à D, il est possible de :

- inventorier l'ancien VPS Oracle et sauvegarder la base et les clés ;
- installer la nouvelle API sur une adresse non annoncée au public ;
- migrer une copie de la base ;
- tester les e-mails avec des destinataires internes ;
- utiliser Stripe en mode test ;
- construire des exécutables non signés clairement réservés à la recette ;
- tester les forks avec `RELAISDESK_AUTH_REQUIRED=N` dans un environnement
  fermé ;
- préparer Nginx, les certificats et le pare-feu sans ouvrir les ventes.

Employer uniquement des données fictives ou une copie protégée de production.
Si des données réelles sont utilisées, les règles RGPD et de sécurité restent
applicables même en préproduction.

## 5. Porte B — avant la bascule technique publique

Cette porte doit être fermée avant de rendre le serveur forké accessible au
public, de publier les téléchargements ou de déplacer les données clients vers
la nouvelle production.

### 5.1 Cartographier les données et les rôles RGPD

Créer un registre simple indiquant, pour chaque traitement :

- les données collectées : identité, e-mail, société, commande, paiement,
  licence, appareil, connexion, journal et historique d'intervention ;
- la finalité et la base juridique ;
- qui est responsable de traitement et qui est sous-traitant ;
- les destinataires, sous-traitants ultérieurs et pays d'hébergement ou d'accès ;
- la durée de conservation, la purge et l'emplacement des sauvegardes ;
- les mesures de sécurité et les personnes autorisées.

RelaisDesk est vraisemblablement responsable de traitement pour la vente, les
comptes, la facturation, la sécurité et la lutte contre la fraude. Pour certaines
métadonnées ou historiques gérés au nom d'un client professionnel, RelaisDesk
peut devenir son sous-traitant. Cette répartition doit être vérifiée au cas par
cas et inscrite dans le contrat.

### 5.2 Encadrer les sous-traitants

- Télécharger et archiver les conditions et annexes de protection des données
  réellement acceptées pour OVH, Oracle Cloud, Stripe et le prestataire SMTP.
- Vérifier la région Oracle, les lieux de support, les sous-traitants ultérieurs
  et les éventuels transferts hors EEE.
- Documenter la garantie utilisée pour chaque transfert : décision d'adéquation,
  cadre applicable ou clauses contractuelles types, selon le contrat réel.
- Préparer une annexe article 28 destinée aux clients B2B lorsque RelaisDesk
  agit comme sous-traitant : objet, durée, catégories de données, instructions,
  confidentialité, sécurité, sous-traitants, assistance, sort des données et
  droit d'audit.

Une liste de fournisseurs dans la politique de confidentialité ne remplace pas
les contrats et annexes article 28.

### 5.3 Sécurité, conservation et incidents

- Valider les sauvegardes chiffrées et un test de restauration.
- Limiter les accès administrateurs, activer l'authentification multifacteur et
  conserver des traces d'administration proportionnées.
- Définir les délais de purge de la base, des journaux et des sauvegardes.
- Écrire une procédure de violation de données : détection, confinement,
  qualification, registre interne, information du client responsable de
  traitement et, si nécessaire, notification à la CNIL et aux personnes.
- Préparer un canal pour l'accès, la rectification, l'effacement, l'opposition,
  la limitation et la portabilité lorsque le droit s'applique.
- N'activer aucun outil d'analyse, publicité ou cookie non nécessaire avant
  d'avoir mis en place l'information et, lorsque requis, le consentement.

### 5.4 Code source réellement exploité

Avant de démarrer publiquement `hbbs`/`hbbr` modifiés, remplacer les marqueurs
de `relaisdesk/logiciel-libre.html`, configurer `CLIENT_SOURCE_URL` et
`SERVER_SOURCE_URL`, et vérifier que ces URL correspondent au commit déployé.

**Sortie de la porte B :** la production peut être exposée sans téléchargement
orphelin de ses sources, les données sont cartographiées, les prestataires sont
encadrés et un retour arrière est prêt.

## 6. Porte C — avant la première vente B2B

### 6.1 Entreprise individuelle et activité

Il n'est pas nécessaire de créer une deuxième entreprise. Depuis le guichet
unique, vérifier l'EI existante et, si besoin :

- ajouter précisément l'activité de fourniture/exploitation du service
  RelaisDesk ;
- déclarer « RelaisDesk » comme nom commercial ou enseigne selon l'usage réel ;
- vérifier que l'adresse, le téléphone et les activités sont à jour au RNE.

Le code APE 9511Z n'interdit pas une activité supplémentaire. L'INSEE peut le
modifier si l'activité principale réelle change ; ne choisir ni ne modifier le
code APE soi-même dans les documents sans confirmation officielle.

### 6.2 Site, offre et contrat B2B

- Finaliser les mentions légales et vérifier les coordonnées réelles d'OVH.
- Rendre les CGV B2B accessibles avant commande, téléchargeables et
  communicables sur demande.
- Décrire exactement le service vendu : durée de trente jours, nombre maximal
  de techniciens **simultanés**, support, maintenance, dépendance au réseau,
  compatibilité et, selon le parcours, durée prépayée sans reconduction ou
  abonnement récurrent après essai.
- Ne pas annoncer le contrôle anti-partage du fork avant son activation réelle.
- Faire accepter sans case précochée la version exacte des CGV et en conserver
  la date, la version et le justificatif.
- Ajouter une annexe de sous-traitance RGPD au contrat lorsque nécessaire.
- Prévoir les règles d'autorisation d'accès distant : le technicien doit avoir
  l'accord du propriétaire ou de l'utilisateur habilité de la machine et ne
  doit pas contourner ses politiques internes.

### 6.3 Facturation et paiement

Chaque facture doit notamment comporter une numérotation chronologique et
continue, les dates, l'identité EI, le SIREN, l'identité du client, la prestation,
les montants, l'échéance, les conditions d'escompte, les pénalités de retard et,
pour un client professionnel, l'indemnité forfaitaire de 40 euros. Conserver la
mention « TVA non applicable, art. 293 B du CGI » tant que la franchise reste
applicable.

Comme l'activité déclarée est artisanale, vérifier avec la CMA ou le conseil
juridique si les références d'assurance doivent figurer sur les factures de la
prestation concernée, puis les ajouter si elles sont applicables.

Au **1er septembre 2026**, une micro-entreprise doit déjà être en mesure de
**recevoir** des factures électroniques B2B par une plateforme agréée. Son
obligation générale d'**émettre** par ce canal commence au 1er septembre 2027.
La modification de l'API d'émission peut donc être planifiée pour l'année
prochaine, mais le choix du canal de réception doit être traité maintenant avec
le comptable, la banque ou une plateforme agréée.

### 6.4 Validation de la vente

- Tester Stripe en production avec une commande interne contrôlée et le webhook
  exact `https://api.relaisdesk.fr/api/v1/stripe/webhook`.
- Vérifier la facture, l'e-mail, la licence, le téléchargement signé, la version
  des CGV et l'absence de doublon lors d'une nouvelle livraison du webhook.
- Vérifier la RC Pro existante et déclarer au besoin l'activité SaaS/accès
  distant à l'assureur ; étudier une garantie cyber.
- Faire relire CGV, limitation de responsabilité, RGPD et annexe article 28 par
  un avocat français avant le premier paiement réel.

**Sortie de la porte C :** les ventes B2B peuvent être ouvertes. Le médiateur de
la consommation n'est pas un préalable à une vente conclue exclusivement avec
un client professionnel.

## 7. Porte D — avant la première vente B2C

Cette porte est indépendante. Conserver `B2C_SALES_ENABLED=false` et l'option
consommateur désactivée tant que tous les points suivants ne sont pas clos.

### 7.1 Médiateur de la consommation

- Choisir un médiateur référencé par la CECMC et compétent pour le service.
- Conclure la convention ou l'adhésion avant de publier ses coordonnées.
- Inscrire son nom, son adresse et son site, de façon visible, sur le site, dans
  les CGV et sur le support de commande ; les rappeler après l'échec d'une
  réclamation préalable.
- Renseigner les variables `CONSUMER_MEDIATOR_NAME` et
  `CONSUMER_MEDIATOR_URL`, puis mettre à jour la copie des CGV jointe aux e-mails.

### 7.2 Parcours de commande consommateur

- Afficher avant paiement les caractéristiques essentielles, le prix total, la
  durée, la compatibilité, les moyens de paiement, l'identité du vendeur, les
  garanties, la rétractation, la médiation et les modalités de réclamation.
- Présenter un récapitulatif permettant de corriger la commande.
- Utiliser un bouton indiquant sans ambiguïté l'obligation de paiement.
- Recueillir l'acceptation des CGV sans case précochée et envoyer sur support
  durable la confirmation, les CGV acceptées et le formulaire de rétractation.
- Si le consommateur souhaite l'activation avant la fin des quatorze jours,
  recueillir sa demande expresse. En cas de rétractation après le début du
  service, seul le montant proportionnel légalement justifié peut être dû ; ne
  pas transformer cette demande en renonciation générale automatique.
- Maintenir un mécanisme de rétractation en ligne fonctionnel et envoyer sans
  délai un accusé de réception durable.
- Décrire et appliquer la garantie légale de conformité des contenus et services
  numériques, les mises à jour nécessaires et les recours.

Deux parcours coexistent : les achats prépayés expirent sans renouvellement
automatique ; les essais ouvrent après 30 jours un abonnement payant sauf
annulation. Le mensuel est facturé au mois calendaire et l'annuel à l'année.
L'information tarifaire et le prélèvement récurrent font l'objet d'une acceptation
expresse. L'espace client permet de notifier l'arrêt des renouvellements après
récapitulatif ; la rétractation est une démarche distincte. Le rappel de fin
d'essai et l'information de reconduction annuelle sont également distincts.
Suivre [EXPLOITATION_ESSAIS_B2C.md](EXPLOITATION_ESSAIS_B2C.md) ; ne pas modifier
les versions contractuelles déjà enregistrées pour cette correction d'interface.

### 7.3 Contrôle final B2C

- Effectuer une commande consommateur complète sur la recette.
- Tester la rétractation avant et après activation immédiate.
- Vérifier l'accusé de réception, la preuve de consentement et le prorata.
- Faire valider l'ensemble par un avocat connaissant le droit français de la
  consommation et les services numériques.
- Seulement après validation, retirer le blocage visuel et définir
  `B2C_SALES_ENABLED=true`.

## 8. Feu vert avant la bascule finale

Ne pas ouvrir la production commerciale tant qu'une case nécessaire au public
visé reste vide.

### Commun à B2B et B2C

- [ ] activité et nom commercial vérifiés au RNE ;
- [ ] identité, hébergeur, contact et TVA vérifiés sur le site ;
- [ ] dépôts publics sans secrets, licences et avis complets ;
- [ ] URL de source correspondant exactement à chaque version distribuée ou
      exploitée ;
- [ ] signature Authenticode décrite honnêtement ; manifeste Ed25519 valide sur les fichiers finaux, avec ou sans SignPath ;
- [ ] CGV et politique de confidentialité finalisées et datées ;
- [ ] registre RGPD, sous-traitants, transferts et durées documentés ;
- [ ] annexe article 28 client disponible lorsque nécessaire ;
- [ ] sauvegarde/restauration et procédure d'incident testées ;
- [ ] factures conformes et plateforme de réception électronique choisie ;
- [ ] Stripe, SMTP, téléchargement et parcours complet testés ;
- [ ] relecture juridique documentée.

### Supplément B2C

- [ ] convention de médiation signée et coordonnées publiées partout ;
- [ ] information précontractuelle et bouton de paiement validés ;
- [ ] confirmation durable avec CGV et formulaire ;
- [ ] rétractation en ligne et activation immédiate testées ;
- [ ] garanties légales numériques vérifiées ;
- [ ] `B2C_SALES_ENABLED=true` activé seulement en dernier.

## 9. Après la migration : preuves et suivi

- Archiver chaque version de CGV acceptée et sa date d'entrée en vigueur.
- Conserver les factures et pièces comptables pendant la durée légale applicable,
  généralement dix ans pour les pièces comptables.
- Conserver pour chaque version logicielle le tag, le commit, les sources, les
  scripts, les licences, les artefacts, les signatures et les SHA-256.
- Réviser le registre RGPD, la liste des sous-traitants et les transferts à
  chaque changement d'OVH, Oracle, Stripe, SMTP ou outil d'analyse.
- Documenter les demandes d'exercice de droits et les incidents.
- Contrôler périodiquement les purges, restaurations, accès administrateurs,
  versions publiques de source et renouvellements de certificat.
- Mettre à jour les CGV avant toute reconduction automatique, nouveau mode de
  paiement, changement substantiel du service ou nouvelle catégorie de données.
- Préparer l'émission de factures électroniques B2B avant le 1er septembre 2027.

## 10. Ce que Julien doit faire personnellement ou avec un professionnel

Le code et les documents peuvent être préparés dans le projet, mais les actions
suivantes nécessitent le titulaire de l'entreprise ou un conseil mandaté :

1. consulter le RNE et déposer une éventuelle formalité de modification ;
2. choisir et contracter avec le médiateur de la consommation ;
3. accepter et archiver les contrats/DPA des fournisseurs ;
4. choisir une plateforme agréée de facturation électronique pour la réception ;
5. informer l'assureur de l'activité et vérifier les garanties ;
6. déposer éventuellement la marque RelaisDesk ;
7. faire valider les CGV, le RGPD et l'annexe article 28 par un avocat ;
8. déposer la candidature SignPath et accepter ses conditions.

## Références officielles

- [Service Public — mentions obligatoires du site d'un entrepreneur individuel](https://entreprendre.service-public.gouv.fr/vosdroits/F31228)
- [Guichet unique — déclaration des établissements, activités et noms commerciaux](https://formalites.entreprises.gouv.fr/etablissements.php)
- [DGCCRF — règles du commerce électronique B2C](https://www.economie.gouv.fr/dgccrf/les-fiches-pratiques/e-commerce-les-regles-entre-professionnels-et-consommateurs)
- [Ministère de l'Économie — obligations relatives au médiateur de la consommation](https://www.economie.gouv.fr/mediation-conso/vous-etes-un-professionnel/vos-principales-obligations-0)
- [Service Public — mentions obligatoires sur une facture](https://entreprendre.service-public.gouv.fr/vosdroits/F31808)
- [Ministère de l'Économie — calendrier de la facturation électronique](https://www.economie.gouv.fr/tout-savoir-sur-la-facturation-electronique-pour-les-entreprises)
- [CNIL — clauses article 28 entre responsable de traitement et sous-traitant](https://www.cnil.fr/fr/clauses-contractuelles-types-entre-responsable-de-traitement-et-sous-traitant)
- [CNIL — cartographier les traitements de données](https://www.cnil.fr/fr/cartographier-vos-traitements-de-donnees-personnelles)
- [CNIL — identifier et encadrer les transferts hors UE](https://cnil.fr/fr/responsables-de-traitement-comment-identifier-et-traiter-des-transferts-de-donnees-hors-ue)
- [GNU — texte de la GNU AGPLv3](https://www.gnu.org/licenses/agpl-3.0.html)
- [SignPath Foundation — conditions du programme open source](https://signpath.org/terms)
