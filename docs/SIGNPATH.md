# Signature Windows avec SignPath Foundation

## Décision et état réel

RelaisDesk retient **SignPath Foundation** comme première voie de signature
Authenticode des binaires Windows. Le programme est gratuit pour les projets
open source acceptés, mais il ne constitue ni un droit ni une signature active
immédiatement. Tant que la candidature n'est pas acceptée et que la chaîne CI
n'a pas produit un artefact signé vérifié, aucune signature Authenticode n'est
revendiquée. **Décision du 7 septembre 2026 : SignPath n'est pas une condition
de distribution.** Les binaires peuvent être publiés non signés, avec
information explicite, sources correspondantes, licences et manifeste Ed25519.

Le certificat appartient à SignPath Foundation : Windows affichera donc
`SignPath Foundation` comme éditeur, et non `RelaisDesk` ou
`Informatique à domicile 03`. La signature établit l'origine du build et son
intégrité, mais ne garantit pas à elle seule l'absence immédiate d'un
avertissement SmartScreen.

Références officielles :

- [programme SignPath Foundation](https://signpath.org/) ;
- [conditions des projets open source](https://signpath.org/terms) ;
- [formulaire de candidature](https://signpath.org/apply) ;
- [intégration GitHub officielle](https://docs.signpath.io/trusted-build-systems/github) ;
- [position de Microsoft sur SignPath et SmartScreen](https://learn.microsoft.com/fr-fr/windows/apps/package-and-deploy/code-signing-options).

## Audit d'éligibilité RelaisDesk au 1er septembre 2026

| Exigence | État | Action avant candidature |
|---|---|---|
| Fork client sous licence OSI | acquis | le fork public `JuLeXoGame/rustdesk` conserve `rustdesk/LICENCE` (AGPLv3) et son historique |
| Fork serveur sous licence OSI | acquis | le fork public `JuLeXoGame/rustdesk-server` conserve `rustdesk-server/LICENSE` (AGPLv3) et son historique |
| `hbb_common` clairement licencié dans son propre fork | acquis | les branches publiques client et serveur contiennent le texte AGPLv3 et leur notice d'attribution |
| Sources des configurateurs, viewers et scripts de build sous licence OSI | bloquant | le dossier principal n'est pas un dépôt Git et `installer/` n'a pas de licence explicite ; publier le périmètre distribué sous une licence OSI validée juridiquement |
| Aucun composant propriétaire dans les artefacts à signer | à démontrer | inventorier toutes les DLL, ressources et binaires embarqués ; publier leurs sources ou documenter leur licence compatible |
| API hébergée nécessaire au fonctionnement | à clarifier | demander à SignPath si elle peut rester un service externe non inclus dans le projet de signature ; sinon publier son code sous licence OSI avant candidature |
| Build vérifiable depuis un dépôt public | partiel | les CI publiques du client et du serveur utilisent des runners GitHub hébergés et conservent leurs sorties comme artefacts, sans publier automatiquement les binaires non signés ; le build complet des launchers et de NSIS doit encore être porté dans la CI publique |
| Projet déjà publié dans la forme à signer | bloquant | créer une préversion publique non signée, clairement marquée comme telle, avec sources et notes de version correspondantes |
| MFA, revues et approbation des signatures | à faire par le propriétaire GitHub | activer la MFA, protéger les branches, déclarer les rôles et imposer l'approbation manuelle de chaque release |
| Politique de signature et confidentialité | à compléter | publier la politique décrite ci-dessous et relier la politique de confidentialité de RelaisDesk |

L'AGPL autorise la vente du service et du support. En revanche, le programme
gratuit SignPath exige que le projet présenté n'utilise pas de double licence
commerciale et ne contienne pas de composant propriétaire. Toute exception ou
incertitude doit être soumise à SignPath par écrit avant de présenter la
candidature comme éligible.

## Périmètre de dépôt recommandé

Créer un dépôt public dédié à la distribution cliente, par exemple
`<COMPTE>/relaisdesk-client-distribution`, contenant au minimum :

- les sources de `configurator`, `viewer`, `debpack` et des installateurs NSIS ;
- les scripts qui construisent les artefacts, sans étape cachée exécutée sur le
  PC local ;
- les références exactes vers les commits des forks `rustdesk` et
  `hbb_common` ;
- la licence OSI choisie pour le code détenu par RelaisDesk, les licences des
  dépendances et les attributions ;
- la politique de confidentialité, les modifications système annoncées et la
  procédure de désinstallation ;
- la politique de signature, les rôles et les notes de version.

Ne jamais publier `api/.env`, une base SQLite, un jeton Stripe, SMTP ou DNS, une
clé Ed25519 privée, `SIGNPATH_API_TOKEN` ni des données client.

## Politique de signature à publier

La page d'accueil du dépôt et chaque page de téléchargement doivent comporter
une section nommée **Code signing policy** avec au minimum :

- la mention `Free code signing provided by SignPath.io, certificate by SignPath Foundation` ;
- le lien vers le dépôt et le commit ayant produit chaque version ;
- les personnes ou équipes assumant les rôles de contributeur, relecteur et
  approbateur de signature ;
- le lien vers la politique de confidentialité ;
- la déclaration des connexions réseau effectuées par le logiciel ;
- l'engagement de ne signer que les artefacts issus de la CI publique validée.

Le dépôt doit aussi expliquer clairement que RelaisDesk est un outil
d'assistance à distance soumis au consentement de l'utilisateur, qu'il ne doit
jamais être installé pour obtenir un accès non autorisé et qu'une désinstallation
complète est fournie.

## Séquence de candidature

1. Conserver les trois forks déjà publiés et publier le dépôt de distribution cliente.
2. Régler les licences manquantes et publier l'inventaire des dépendances.
3. Activer la MFA et les protections de branche GitHub.
4. Faire produire par des runners GitHub hébergés une préversion publique non
   signée, avec un tag et ses sommes SHA-256.
5. Publier la politique de signature et les pages de confidentialité et de
   désinstallation.
6. Déposer la candidature sur `https://signpath.org/apply` avec les URL des
   dépôts, de la préversion et de la politique.
7. Attendre l'acceptation et les identifiants de projet fournis par SignPath
   avant d'ajouter une étape de signature à la CI.

Ne pas acheter de certificat EV pendant l'étude du dossier. En cas de refus
définitif, la solution de repli documentée est Azure Artifact Signing, puis un
certificat OV ; EV n'est pas requis pour SmartScreen.

## Intégration CI après acceptation uniquement

SignPath devra approuver la configuration des artefacts et les fichiers à
signer. Le workflow utilisera uniquement des runners GitHub hébergés. Il devra :

1. construire le fork RustDesk non signé ;
2. téléverser cet artefact avec `actions/upload-artifact` ;
3. soumettre la première demande SignPath avec
   `signpath/github-action-submit-signing-request@v2` ;
4. construire les configurateurs et viewers en embarquant **le RustDesk déjà
   signé**, afin que leurs empreintes intégrées correspondent au bon fichier ;
5. soumettre les configurateurs et viewers à SignPath ;
6. construire les installateurs NSIS à partir des exécutables internes signés ;
7. soumettre les installateurs finaux à SignPath ;
8. vérifier toutes les signatures, puis générer en dernier le manifeste
   Ed25519 et `SHA256SUMS.txt` sur les fichiers définitifs.

Le secret GitHub `SIGNPATH_API_TOKEN` ne sera créé qu'après acceptation, avec le
minimum de droits. Les identifiants `organization-id`, `project-slug`,
`signing-policy-slug` et `artifact-configuration-slug` devront reprendre
exactement les valeurs fournies dans le compte SignPath. Ne jamais inventer ces
valeurs ni copier un jeton dans le YAML.

L'action officielle doit être épinglée à un commit relu avant la production,
même si la documentation SignPath l'illustre avec la branche majeure `@v2`.
Chaque signature de release reste soumise à une approbation manuelle.

## Contrôles avant publication

Sur le poste Windows, télécharger l'artefact final depuis le workflow GitHub,
pas depuis un dossier local, puis exécuter :

```powershell
Get-ChildItem C:\build\relaisdesk-signed -Recurse -Include *.exe,*.dll |
  Get-AuthenticodeSignature |
  Format-Table Path, Status, @{Name='Publisher';Expression={$_.SignerCertificate.Subject}}
```

Tous les fichiers Windows distribués doivent avoir le statut `Valid` et
l'éditeur attendu de SignPath Foundation. Extraire ou installer dans une VM
Windows propre et vérifier également les exécutables réellement déposés sur le
disque. Comparer ensuite les SHA-256 au manifeste Ed25519 final avant le
téléversement vers OVH et Oracle.
