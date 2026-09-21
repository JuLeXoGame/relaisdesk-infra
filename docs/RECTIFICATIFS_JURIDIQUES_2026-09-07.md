# Corrections juridiques et déploiement du 7 septembre 2026

**État actualisé le 8 septembre : API et téléchargements déployés sur Oracle,
site OVH constaté à jour.** Voir [DEPLOIEMENT_ORACLE_2026-09-08.md](DEPLOIEMENT_ORACLE_2026-09-08.md)
pour les contrôles et les sauvegardes. Aucun envoi ANSSI n'a été fait.

## Décisions prises

- Les tarifs du site font référence : Starter 24,90 EUR/30 jours ou 239 EUR/365 jours ; Pro 129 EUR/30 jours ou 1 290 EUR/365 jours, jusqu'à 5 techniciens ; Personnalisé 10 à 500 techniciens, tarif affiché avant paiement.
- Les postes installés ne sont pas limités. La capacité porte sur les postes techniciens connectés simultanément.
- `eu-marseille-1` existe : région commerciale OCI France Sud (Marseille), et non Oracle EU Sovereign Cloud. Le site demeure chez OVH ; aucun déplacement d'infrastructure n'est prévu.
- Les ventes B2C restent fermées, médiateur différé. Ne pas activer `B2C_SALES_ENABLED` avant sa désignation et la mise à jour des documents.
- SignPath n'est plus une condition de distribution. Ne jamais annoncer une signature Authenticode inexistante. Le manifeste Ed25519 reste obligatoire dans le circuit de téléchargement sécurisé.
- L'email professionnel est public et accessible, avec obfuscation HTML/JavaScript et repli sans JavaScript. Aucune technique côté navigateur ne peut garantir qu'un robot avancé ne le collectera pas.

## Livraison réalisée et limites

- Dépôt client : commit `b2e236e186c2abe119da8869629b43847104308a`, documentation de construction/source, script Sciter autonome, contact sécurité, correction du checkout des sous-modules dans le workflow de certificats racines.
- Dépôt serveur : commit `0f816b56d221867d15027fac0a1a654367f0ffb3`, documentation publique de construction/autorisation et politique de sécurité ; lien auparavant hors du dépôt corrigé.
- Les branches principales des trois forks sont protégées contre les force-push et suppressions, y compris pour les administrateurs. Les écritures normales restent possibles ; aucune deuxième personne ni validation CI obligatoire n'a été imposée. Cela n'est pas une revue automatique de chaque commit.
- Alertes et propositions de correctifs Dependabot activées sur les trois forks, sans fusion automatique. **35 signalements initiaux nécessitent une qualification technique séparée** : voir [TRIAGE_DEPENDABOT_2026-09-07.md](TRIAGE_DEPENDABOT_2026-09-07.md). Ne pas présenter ces réglages comme une correction des dépendances.
- Région du serveur confirmée en lecture seule par ses métadonnées OCI : `oracle`, `eu-marseille-1`. La correction concerne bien la distinction entre OCI commercial et EU Sovereign Cloud, pas l'existence de Marseille.
- Nouvelle API Linux/amd64 compilée et deux installateurs NSIS reconstruits. Les moteurs client et serveur RustDesk ne sont pas recompilés dans ce lot juridique.
- Paquet local désormais archivé dans `archives/livraisons/deployment-2026-09-07/` : ZIP de 13 fichiers OVH, binaire Oracle, téléchargements complets et nouveau manifeste `1.0.0-legal.20260907`, signé avec la clé existante et vérifié indépendamment. Déplacement de rangement uniquement ; contenu conservé.
- Clé privée retrouvée localement : `C:\RelaisDesk-Secrets\release-signing-ed25519`. Elle n'a pas été affichée, copiée dans le paquet, remplacée ni publiée.
- Au terme de la préparation initiale du 7 septembre, OVH et Oracle n'avaient pas été modifiés. Sur demande de l'exploitant, l'API et les téléchargements ont ensuite été déployés sur Oracle le 8 septembre, après constat de la mise à jour OVH. Aucun document n'a été envoyé à l'ANSSI.

Les références immuables des binaires existants restent celles du 6 septembre
dans la page logiciel libre et les rectificatifs ANSSI. Les nouveaux commits
documentaires ne doivent pas leur être substitués comme preuve de compilation.

## Documents à renvoyer à l'ANSSI

Répondre au courriel du dossier **déjà envoyé**, en conservant sa référence.
Ne pas envoyer les documents rectifiés comme un dossier indépendant ni
modifier les originaux signés du 5 septembre.

Le dossier `output/pdf/anssi-rectificatifs-2026-09-07/` contient :

1. `01_Addendum_formulaire_ANSSI_2026-09-07.pdf` : rectification des cadres A, B et C du formulaire. Compléter la référence du dossier et la date du premier envoi, relire puis signer.
2. `02_Note_technique_rectifiee_2026-09-07.pdf` : remplace la précédente note technique. Relire puis dater et signer dans l'espace prévu.
3. `03_Brochure_commerciale_rectifiee_2026-09-07.pdf` : remplace la brochure commerciale (tarifs, capacités, B2B actuel, limites des affirmations de sécurité).
4. `04_Presentation_EI_rectifiee_2026-09-07.pdf` : remplace la présentation de l'exploitant (EI, hébergement, portée des affirmations RGPD).
5. **A obtenir vous-même :** justificatif d'immatriculation RNE récent, ou document demandé par le bureau instructeur pour votre EI. L'avis SIRENE n'est pas un Kbis équivalent. Ne pas fabriquer un justificatif ni déduire la liste des activités du seul code APE.

Le manuel utilisateur n'a pas à être remplacé du seul fait de ce complément,
sauf information devenue inexacte ou demande de l'instructeur. L'avis SIRENE
reste dans l'historique ; expliquer sa portée plutôt que l'effacer.

L'addendum est un complément proposé : demander explicitement si l'ANSSI exige
un **formulaire officiel complet ressaisi**. Dans ce cas, joindre le formulaire
électronique rempli sauvegardé ET son exemplaire signé scanné, comme prévu par
la procédure officielle. Les PDF préparés ici ne sont pas des attestations
de l'ANSSI et aucune signature ancienne n'y est reproduite.

### Courriel proposé (à compléter et envoyer par l'exploitant)

Objet : `[formalités] RelaisDesk - Complément rectificatif au dossier [référence]`

Madame, Monsieur,

Faisant suite à mon envoi du [date] concernant RelaisDesk [référence éventuelle],
je vous adresse un addendum au formulaire et trois pièces rectifiées qui
remplacent leurs versions datées du 5 septembre 2026.

Les corrections concernent la qualité d'entrepreneur individuel, les versions
distinctes client/serveur, les primitives cryptographiques réellement utilisées,
la portée de la demande de classement grand public, l'hébergement et la
présentation commerciale actuelle. Les documents signés initiaux restent
identifiés dans l'historique du dossier.

[Joindre le justificatif RNE disponible, ou préciser qu'il suivra / demander
le justificatif attendu pour une EI. Supprimer cette instruction du message.]

Merci de rattacher ces pièces au dossier initial et de m'indiquer si vous
souhaitez un formulaire officiel entièrement ressaisi, en complément de
l'addendum. Je vous remercie de confirmer la prise en compte des rectifications.

Julien BELLOT, entrepreneur individuel
RelaisDesk / Informatique A Domicile 03

## Site, API et installateurs : mise en ligne coordonnée

Les modifications locales ne changent pas automatiquement OVH, Oracle ou les
binaires déjà téléchargés. Prévoir une courte fenêtre coordonnée pour le site
et l'API : un onglet resté sur l'ancienne version doit être rechargé ; l'API ne
doit pas enregistrer une nouvelle commande avec les anciennes conditions.

1. Sauvegarder le code déployé, les documents contractuels en vigueur et la base
   SQLite avec l'outil de sauvegarde habituel. Ne pas remplacer la base par une
   base de développement. Ne pas modifier les versions enregistrées des commandes.
2. Déployer les pages modifiées, `sous-traitance-rgpd.html`, `legal-email.js`,
   `app.js`, `client/app.js`, `i18n.js` et `styles.css` **chez OVH**. Conserver les
   règles `.htaccess` existantes de redirection des téléchargements vers l'API.
3. Recompiler l'API avec les deux archives intégrées puis remplacer son binaire
   sur Oracle selon la procédure existante. Conserver l'environnement réel et
   `B2C_SALES_ENABLED=false`. Aucun nouveau secret ni changement DNS n'est requis.
4. Vérifier santé API, commande B2B en recette, prix/durée et acceptation de la
   version actuelle. L'exploitant confirme qu'aucune commande n'a déjà été
   acceptée : aucune reprise d'anciennes commandes n'est requise pour ce déploiement.
5. Les deux installateurs NSIS ont été recompilés dans le paquet de préparation.
   Leurs notices et textes AGPL embarqués ont été extraits et comparés aux
   sources. Les portables et paquets Linux y sont repris sans modification.
6. Déployer l'ensemble des téléchargements et leur **nouveau manifeste signé**
   de façon coordonnée, avec sauvegarde et possibilité de retour arrière.
   Les noms sont conservés pour la compatibilité ; prévoir une courte maintenance
   pour ne pas servir une nouvelle empreinte avec un ancien fichier, ou l'inverse.
   Le manifeste `1.0.0-legal.20260907` est déjà signé avec la clé attendue en
   production. Toute modification ultérieure d'un artefact impose de le refaire.
7. Archiver les paquets, le manifeste et une fiche de provenance par version :
   commits client/serveur/sous-modules, scripts, versions des outils et empreintes.
   Un commit documentaire plus récent ne signifie pas que les binaires ont été
   reconstruits à partir de lui. Conserver les anciens liens immuables.

## Tests de cohérence

Depuis la racine, avec Node et Python disponibles :

```powershell
node scripts/legal-checks.mjs
python scripts/legal_contract_export.py
```

Puis exécuter `go test ./...` dans `api`, `database` et `tests`.
`legal_contract_export.py --patch` ne sert qu'à créer une nouvelle archive :
il refuse d'écraser une archive différente. L'archive du 25 août est conservée
octet pour octet (SHA-256 `8f8dfdda5f02f57089dfe38a4b9caa684b47d916646a8e74ac38d87b3dea1be2`).

Résultats de cette livraison : les trois suites Go, les contrôles Node et
l'export contractuel passent. ZIP OVH vérifié contre les 13 sources ; notices
des deux installateurs extraites et comparées ; manifeste Ed25519 et empreintes
des 7 artefacts vérifiés indépendamment avec Node, y compris un test négatif
d'altération. La syntaxe du script Sciter est validée, mais une construction
complète de cette variante et une installation Windows interactive n'ont pas
été effectuées pour ce lot documentaire.

- Workflow certificats racines : https://github.com/JuLeXoGame/relaisdesk/actions/runs/34164638663 (réussi).
- CI serveur : https://github.com/JuLeXoGame/rustdesk-server/actions/runs/34164551773 (réussie).

## Obligations opérationnelles restant à justifier

- Confirmer les contrats et garanties effectivement applicables chez Oracle,
  OVHcloud et Stripe ; conserver leurs versions et les preuves de localisation.
- Mettre en œuvre l'annexe RGPD : instructions, registre, droits, audits,
  notification d'incident, sous-traitants et effacement/restauration.
- Vérifier au RNE l'activité complémentaire et les noms utilisés, ainsi que la
  couverture d'assurance adaptée. Une page web ne réalise pas ces démarches.
- Tenir la procédure `PROCEDURE_INCIDENTS_CRA_RGPD.md` ; faire qualifier le
  périmètre CRA du produit. Les notifications de l'article 14 s'appliquent aux
  fabricants concernés dès le 11 septembre 2026.
- Faire valider les engagements contractuels par un professionnel du droit.
  Le médiateur, la signature des rectificatifs et les déclarations ne peuvent
  pas être remplacés par une modification du code.

## Références

- ANSSI : https://cyber.gouv.fr/reglementation/reglementation-identite-confiance-numerique/controles-reglementaires-cryptographie/controle-moyen-de-cryptologie/
- Email professionnel, LCEN art. 19 : https://www.legifrance.gouv.fr/loda/article_lc/LEGIARTI000032236011
- Encadré réglementaire : https://www.legifrance.gouv.fr/codes/article_lc/LEGIARTI000045981322
- RGPD art. 28 : https://www.cnil.fr/fr/reglement-europeen-protection-donnees/chapitre4
- Oracle : https://docs.oracle.com/fr-fr/iaas/Content/sovereign-cloud/eu-sovereign-cloud.htm
