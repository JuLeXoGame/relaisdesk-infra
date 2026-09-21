# RelaisDesk face aux offres payantes RustDesk

**Note de décision — 26 août 2026**  
**Périmètre :** version locale actuelle du projet RelaisDesk, comparée aux offres et à la documentation officielles RustDesk consultées le 26 août 2026.  
**Décision visée :** formuler une proposition de valeur honnête, choisir les segments à cibler et éviter les promesses commerciales fragiles.

## Réponse courte

RelaisDesk n’apporte pas « davantage de fonctions que RustDesk Pro » au sens général. Il apporte surtout **une autre couche de produit** : la commercialisation et l’exploitation d’un service d’assistance à distance pour des techniciens, TPE et petites équipes de support.

RustDesk Pro est aujourd’hui plus complet pour administrer un parc : comptes et groupes, 2FA, OIDC, LDAP, carnet d’adresses, politiques centralisées, rôles, inventaire des appareils, journaux techniques, relais multiples, déploiement en masse, générateur de clients et client Web. RelaisDesk est plus adapté lorsqu’il faut vendre une capacité d’assistance, accueillir un client occasionnel sans compte, limiter uniquement les postes techniciens actifs simultanément, facturer, renouveler, relancer et restituer un historique professionnel d’intervention.

La bonne promesse n’est donc pas « un RustDesk Pro moins cher et plus complet », mais :

> **RelaisDesk transforme le socle libre RustDesk en service français d’assistance à distance commercialisable : parcours client temporaire, capacité technicien simultanée, espace client, paiements, factures, renouvellements et historique d’intervention.**

## Conclusions décisives

1. **Le différenciateur principal est commercial, pas le bureau à distance.** Le moteur de connexion reste RustDesk Community forké. RelaisDesk ajoute la vente B2B/B2C, Stripe et virement, factures, CGV, espace client, renouvellements et rappels.
2. **Le modèle d’usage est adapté aux prestataires de support.** Une licence RelaisDesk limite les appareils techniciens actifs simultanément, pas le nombre total d’installations inactives. Les Viewers temporaires sont annoncés comme illimités et expirent après douze heures.
3. **Le parcours d’assistance occasionnelle est plus intégré.** Le technicien génère un code Viewer de douze heures ; le premier appareil l’utilisant y est lié par une clé Ed25519 ; le Viewer obtient sa configuration et son autorisation réseau sans compte commercial permanent.
4. **L’espace client commercial est une vraie différence.** Le titulaire consulte licences, commandes, factures et interventions, renouvelle explicitement et gère les rappels par lien magique. La documentation officielle RustDesk consultée décrit une console d’administration technique, pas un portail de revente destiné aux clients finaux du prestataire.
5. **L’historique RelaisDesk répond au métier de service.** Un code Viewer crée une fiche ; le technicien peut la démarrer, la clôturer et saisir un compte rendu ; le client peut consulter et exporter cet historique. RustDesk Pro possède toutefois un journal d’audit technique beaucoup plus riche.
6. **Le modèle Customized V2 de RustDesk réduit l’exclusivité de l’argument “support occasionnel”.** RustDesk permet déjà de ne pas désactiver les appareils excédentaires et de limiter les connexions simultanées. L’option `register-device=N` évite aussi l’enregistrement d’un appareil, mais supprime pour celui-ci la connexion au compte, l’affectation, les journaux d’audit et les stratégies, et exige une licence Custom2 répondant à certaines conditions.
7. **La sécurité RelaisDesk est spécialisée, pas globalement supérieure.** RelaisDesk ajoute des jetons réseau courts signés, une preuve de possession, l’anti-rejeu, des rôles technicien/Viewer et l’isolation par locataire directement dans `hbbs`/`hbbr`. RustDesk Pro reste supérieur pour l’identité et la gouvernance centralisées (2FA, OIDC, LDAP, rôles, politiques, audit).
8. **RelaisDesk est encore en préproduction.** Les sources des forks ne sont pas encore publiées, des artefacts sont non signés, les builds Linux/CI et la recette réseau complète restent à terminer. Il serait prématuré de promettre la maturité opérationnelle ou le support d’un éditeur établi.

## Comparaison fonctionnelle

| Domaine | RelaisDesk — projet local | RustDesk Server Pro — offre actuelle | Lecture commerciale |
|---|---|---|---|
| Positionnement | Plateforme d’assistance à distance à vendre à des techniciens et petites équipes | Plateforme auto-hébergée d’administration centralisée d’utilisateurs et d’appareils | Différence de métier, non simple liste de fonctions |
| Commerce | Souscription, Stripe, virement, facturation, B2B/B2C, acceptation des CGV | Achat de la licence RustDesk par l’exploitant ; aucune pile de revente aval identifiée dans les pages officielles étudiées | **Avantage RelaisDesk** |
| Espace client | Portail sans mot de passe : licences, commandes, factures, renouvellements, préférences, interventions | Console Web pour utilisateurs, appareils, groupes, permissions, stratégies, relais, journaux et clients personnalisés | Les deux portails n’ont pas le même utilisateur ni le même objectif |
| Licence | Capacité d’appareils techniciens actifs simultanément ; installations inactives non décomptées | Plans classiques : utilisateurs connectés + appareils gérés, connexions illimitées ; V2 : appareils excédentaires non désactivés + canaux simultanés limités | **Avantage RelaisDesk en simplicité**, mais V2 chevauche fortement ce modèle |
| Assistance occasionnelle | Code Viewer 12 h, pas de compte client, configuration automatique, liaison au premier appareil | Client Quick Support et modèle Custom2 ; `register-device=N` possible avec restrictions | **Avantage RelaisDesk pour le parcours intégré**, pas pour l’existence du cas d’usage |
| Isolation commerciale | Locataire stable par client ; rôles technicien/Viewer ; connexion autorisée seulement dans le même locataire | ACL par utilisateurs, groupes et groupes d’appareils ; rôles de contrôle et d’administration | RelaisDesk est simple et orienté abonnement ; Pro est plus granulaire |
| Sécurité d’autorisation réseau | Jeton Ed25519 de 5 min, preuve de possession par appareil, nonce anti-rejeu, révocation propagée, quota contrôlé dans `hbbs`/`hbbr` | Comptes, 2FA e-mail/TOTP, OIDC/LDAP, contrôle d’accès, politiques de sécurité, appareils de confiance | Aucun vainqueur absolu ; modèles de risque différents |
| Historique | Fiche professionnelle, durée, référence client, objet, compte rendu, statut, export CSV accessible au client | Audits techniques des connexions, transferts de fichiers, alarmes et opérations console | RelaisDesk gagne sur la restitution métier ; Pro gagne sur l’audit technique |
| Parc et politiques | Pas d’équivalent complet actuel à l’inventaire, groupes d’appareils, carnets d’adresses et stratégies Pro | Gestion d’appareils, groupes, carnets d’adresses, stratégies synchronisées, accès croisés | **Avantage net RustDesk Pro** |
| Identité d’entreprise | Authentification technicien par licence ; lien magique côté client commercial | Comptes, 2FA, OIDC SSO et LDAP ; rôles administratifs délégués | **Avantage net RustDesk Pro** |
| Déploiement | Launchers RelaisDesk, configuration serveur imposée, forks Windows/Linux prévus, manifeste de version signé | Générateur de client personnalisé, Windows/macOS/Linux/Android, scripts, MSI, RMM/Intune/GPO | **Avantage RustDesk Pro** aujourd’hui |
| Relais et Web | Un couple `hbbs`/`hbbr` direct ; ports WebSocket fermés tant qu’aucun client Web n’est utilisé | Relais multiples avec sélection du plus proche ; WebSocket et client Web selon licence | **Avantage RustDesk Pro** |
| Maîtrise du produit | Forks AGPL contrôlés, feuille de route et modèle commercial propres | Dépendance à l’éditeur et à sa licence commerciale, mais maintenance assurée par lui | Liberté RelaisDesk contre charge de maintenance |
| Support et maturité | Support à organiser par l’entreprise ; préproduction | Support dédié inclus dans tous les plans payants ; produit commercial existant | **Avantage RustDesk Pro** aujourd’hui |

## Ce que RelaisDesk apporte réellement de plus

### 1. Une chaîne commerciale intégrée

Le projet relie une commande à un client stable, une licence, un paiement, une facture et un renouvellement. Le prix est recalculé côté serveur. Stripe active la commande après webhook signé ; un virement peut être validé administrativement. Le portail permet de télécharger les factures et d’initier un renouvellement d’un mois.

Les rappels sont envoyés aux paliers J-14, J-7, J-3, J-1 puis après expiration, avec déduplication persistante. Il n’existe pas de prélèvement récurrent ni de reconduction tacite dans la version actuelle : il faut parler de **renouvellement guidé avec relances automatiques**, et non de renouvellement automatique.

RustDesk indique de son côté une facturation annuelle, sans renouvellement automatique, avec un rappel quatorze jours avant expiration. Cette relation concerne toutefois l’achat de Server Pro par son exploitant, et non la revente de prestations de support aux propres clients de cet exploitant.

### 2. Une unité de vente facile à expliquer

RelaisDesk vend une capacité d’usage simultané par appareil technicien. Le logiciel peut être installé sur plusieurs postes ; seuls ceux qui maintiennent une présence active auprès de `hbbs` consomment un créneau. Ce modèle correspond bien à une petite entreprise ayant de nombreux postes possibles mais peu de techniciens actifs en même temps.

Tarifs encodés dans le projet : Starter, 10 € pour 1 technicien simultané ; Pro, 20 € jusqu’à 10 ; Ultra, progression par tranches au-delà de 10. Les Viewers sont annoncés comme illimités, chaque code durant douze heures.

RustDesk propose aujourd’hui : Individual à 11,88 $/mois facturés annuellement (1 utilisateur connecté, 20 appareils gérés, connexions simultanées illimitées) ; Basic à 23,88 $/mois facturés annuellement (10 utilisateurs, 100 appareils gérés) ; Customized à partir de 23,88 $ avec suppléments ; Customized V2 à partir de 23,88 $, puis 1,20 $ par utilisateur, 0,12 $ par appareil et 24 $ par connexion simultanée supplémentaire.

Il est impossible de conclure honnêtement que RelaisDesk est toujours moins cher : il faut ajouter l’hébergement, le trafic de relais, les sauvegardes, la signature de code, l’assistance, la maintenance des forks et les obligations de conformité. En revanche, **son unité de valeur est mieux alignée sur l’assistance occasionnelle**.

### 3. Un parcours client temporaire et lié à l’intervention

Le code Viewer de douze heures n’est pas seulement un mot de passe RustDesk. Dans RelaisDesk, il sert à récupérer une autorisation réseau limitée, à isoler le client dans le bon locataire, à lier le code au premier appareil et à créer automatiquement une fiche d’intervention. Cette continuité entre accès technique et dossier professionnel est la différence la plus défendable.

RustDesk Pro couvre déjà le Quick Support et les appareils non enregistrés. L’avantage RelaisDesk doit donc être formulé comme **l’orchestration complète et spécialisée** du parcours, non comme une capacité technique absente chez RustDesk.

### 4. Une isolation de licence appliquée au niveau réseau

Le fork RelaisDesk vérifie l’autorisation dans les serveurs de rendez-vous et de relais eux-mêmes. Les jetons courts contiennent le rôle, le locataire, la clé publique de l’appareil, l’expiration et le quota. Chaque opération sensible nécessite une preuve signée liée à l’action, l’heure et un nonce. Un technicien ne peut initier qu’une connexion vers un Viewer du même locataire.

Cela apporte une barrière anti-partage et multi-client spécifique au modèle RelaisDesk. Ce n’est cependant pas une DRM absolue : un utilisateur contrôlant sa machine et modifiant un client libre peut tenter de contourner les coupures locales. Le serveur bloque toute nouvelle inscription ou connexion après expiration/révocation, mais ne peut pas physiquement arracher un flux P2P déjà établi à un client hostile ayant supprimé le canal de santé.

### 5. Un dossier d’intervention visible par le client commercial

La fiche enregistre une référence client, un objet, un statut, un début, une fin, une durée et un compte rendu ; elle ne copie ni écran, ni fichier, ni mot de passe, ni contenu de session. Le technicien et le titulaire commercial disposent donc d’une trace utile pour le service après-vente, la justification d’une prestation ou le suivi d’un dossier.

RustDesk Pro conserve des audits plus détaillés pour la sécurité et l’administration technique. RelaisDesk ne doit pas revendiquer un audit de conformité supérieur ; il offre un **historique métier complémentaire**.

## Là où RustDesk Pro reste supérieur

- Gestion centralisée des utilisateurs, groupes d’utilisateurs, appareils, groupes d’appareils et carnets d’adresses.
- Contrôles d’accès croisés, rôles de contrôle pendant la session et rôles administratifs délégués.
- 2FA e-mail ou TOTP, codes de secours, OIDC et LDAP.
- Stratégies de configuration et de sécurité synchronisées sur les appareils.
- Journaux d’audit techniques : connexion, transfert de fichiers, alarme et console.
- Relais distribués et choix automatique du relais le plus proche.
- Générateur de clients personnalisés et chaîne de déploiement documentée pour Windows, macOS, Linux, Android, MSI, RMM, Intune et GPO.
- Client Web et WebSocket selon les conditions de licence.
- Support dédié inclus et maturité commerciale/plateforme supérieure.

Pour une DSI interne qui veut administrer un parc permanent, appliquer des politiques, connecter Entra ID/LDAP et auditer les actions, RustDesk Pro est actuellement le choix le plus complet. Pour un prestataire qui vend des interventions ponctuelles à beaucoup de clients et veut payer/limiter selon le nombre de techniciens effectivement actifs, RelaisDesk a un meilleur ajustement métier.

## Promesses commerciales recommandées

### Formulations solides

- « Une solution d’assistance à distance pensée pour les techniciens indépendants et les équipes de support. »
- « Installez l’application technicien sur plusieurs postes ; votre offre limite uniquement les postes techniciens actifs simultanément. »
- « Donnez à votre client un code Viewer temporaire de douze heures, sans création de compte. »
- « Retrouvez licences, factures, renouvellements et historiques d’intervention dans un espace client dédié. »
- « Les connexions sont isolées par client et autorisées par des jetons courts liés à l’appareil. »
- « Basé sur RustDesk libre, avec flux direct P2P ou relais lorsque nécessaire. »

### Formulations à éviter

- « Plus sécurisé que RustDesk Pro » : non démontrable globalement.
- « Plus complet que RustDesk Pro » : faux pour la gestion de parc et l’identité d’entreprise.
- « Appareils illimités » sans préciser « Viewers temporaires / installations non simultanément actives ».
- « Renouvellement automatique » : la version actuelle exige une action et un paiement exprès.
- « Aucun équivalent chez RustDesk » pour le support occasionnel ou la concurrence simultanée : Customized V2 existe.
- « Prêt pour la production » avant signature des exécutables, publication des sources correspondantes, builds CI et recette réseau complète.

## Segments à cibler

### Cœur de cible

- techniciens indépendants et artisans informatiques ;
- TPE de dépannage et de maintenance informatique ;
- petites équipes de support externes servant de nombreux clients occasionnels ;
- structures qui veulent une expérience française, une facturation intégrée et un historique client sans déployer une gestion de parc d’entreprise.

### Cibles secondaires

- MSP légers dont les besoins permanents de parc restent limités ;
- organismes ou associations souhaitant commercialiser ou tracer des créneaux d’assistance ponctuelle.

### Mauvais ajustement actuel

- DSI ayant besoin d’Entra ID/OIDC, LDAP et 2FA centralisée ;
- grands parcs permanents avec politiques, groupes, inventaire et carnets d’adresses ;
- organisations exigeant un client Web, plusieurs relais géographiques et une traçabilité technique exhaustive ;
- clients demandant des engagements de support éditeur ou une certification opérationnelle déjà établie.

## Positionnement proposé

**Catégorie :** plateforme française de commercialisation et d’exploitation de l’assistance à distance, basée sur RustDesk libre.

**Phrase courte :**

> **RelaisDesk permet aux professionnels de vendre, sécuriser et tracer leurs interventions à distance, sans facturer chaque PC client installé.**

**Preuve :** codes Viewers temporaires liés à l’appareil, quota technicien simultané contrôlé dans le réseau, espace client commercial, paiements/factures, renouvellements guidés et compte rendu d’intervention.

## Priorités produit suggérées à partir de l’écart concurrentiel

1. **Finir la mise en production avant d’élargir la promesse :** publier les forks et sources correspondantes, exécuter la CI, produire les builds Linux, signer les exécutables Windows et réaliser la recette P2P/relais/révocation/quota.
2. **Ajouter la 2FA aux comptes sensibles RelaisDesk :** au minimum pour l’administration et les techniciens ; c’est le principal écart de sécurité de compte face à Pro.
3. **Enrichir l’historique métier sans copier l’audit Pro :** signature/validation du compte rendu, PDF d’intervention, filtres et export par période/client.
4. **Clarifier publiquement la licence :** “postes techniciens actifs simultanément”, définition d’une présence active, comportement lors d’une coupure et distinction entre Viewer temporaire et appareil géré.
5. **Ne construire un inventaire de parc que si le marché le réclame :** ce serait coûteux et placerait RelaisDesk en concurrence frontale avec la force principale de RustDesk Pro.

## Niveau de confiance et limites

Confiance élevée sur les fonctions RustDesk listées et les tarifs affichés, car les sources officielles ont été consultées le jour de l’étude. Confiance élevée sur les fonctions RelaisDesk vérifiées dans le code et la documentation locale. Confiance moyenne sur l’absence de fonctions commerciales chez RustDesk : la conclusion signifie qu’elles ne sont pas présentées dans les pages officielles de tarification, de Server Pro et de console étudiées ; elle ne prétend pas exclure une intégration tierce ou un développement privé.

Le comparatif porte sur le produit et le positionnement. Il ne constitue ni une consultation juridique, ni une certification de sécurité, ni une validation de production.

## Sources officielles RustDesk

- **S1 — Tarification et FAQ commerciale :** https://rustdesk.com/pricing/?lang=en
- **S2 — Vue d’ensemble de RustDesk Server Pro :** https://rustdesk.com/docs/en/self-host/rustdesk-server-pro/
- **S3 — Console Web :** https://rustdesk.com/docs/en/self-host/rustdesk-server-pro/console/
- **S4 — Configuration et génération de clients :** https://rustdesk.com/docs/en/self-host/client-configuration/
- **S5 — Paramètres avancés, dont `register-device=N` :** https://rustdesk.com/docs/en/self-host/client-configuration/advanced-settings/
- **S6 — Déploiement des clients :** https://rustdesk.com/docs/en/self-host/client-deployment/
- **S7 — Authentification 2FA :** https://rustdesk.com/docs/en/self-host/rustdesk-server-pro/2fa/
- **S8 — Contrôle d’accès :** https://rustdesk.com/docs/en/self-host/rustdesk-server-pro/permissions/
- **S9 — Rôles de contrôle :** https://rustdesk.com/docs/en/self-host/rustdesk-server-pro/control-role/
- **S10 — Stratégies centralisées :** https://rustdesk.com/docs/en/self-host/rustdesk-server-pro/strategy/
- **S11 — LDAP :** https://rustdesk.com/docs/en/self-host/rustdesk-server-pro/ldap/
- **S12 — Définitions des appareils/utilisateurs/connexions et modèle casual support :** https://github.com/rustdesk/rustdesk/wiki/FAQ#what-are-managed-devices--login-users--concurrent-connections-in-pro
- **S13 — Réponse officielle sur Customized V2 et support occasionnel :** https://github.com/rustdesk/rustdesk-server-pro/discussions/182

## Sources locales RelaisDesk

- `README.md`
- `docs/ARCHITECTURE.md`
- `docs/SECURITY.md`
- `docs/FORK_AUTHORIZATION.md`
- `docs/FORK_IMPLEMENTATION_STATUS.md`
- `database/orders.go`
- `database/viewer.go`
- `database/customer_commercial.go`
- `api/handlers/public_orders.go`
- `api/handlers/customer.go`
- `api/handlers/interventions.go`
- `relaisdesk/client/index.html`
- `relaisdesk/cgv.html`
