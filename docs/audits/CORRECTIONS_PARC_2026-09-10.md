# Corrections de la console de parc — 10 septembre 2026

## État de livraison

Corrections effectuées dans les sources locales uniquement. Aucune connexion à Oracle, aucune mise à jour du serveur, aucun envoi GitHub, aucun nouveau binaire publié ou ajouté aux téléchargements du site. Aucune archive de livraison créée.

Les tests automatisés passent. **L'installation réelle du nouveau service Windows et une session distante entre deux PC restent à recetter.** Le code du service n'a pas été exécuté sur le PC de développement.

Complément Linux ajouté à la demande de l'exploitant : l'intégration permanente systemd et le canal local de signature sont maintenant écrits. Voir [le guide Linux](../guides/ACCES_PERMANENT_LINUX.md). Ils nécessitent un nouveau Viewer et un nouveau fork client Linux ; aucune publication n'a été faite. La compilation Linux du Viewer et les tests partagés passent, mais l'exécution réelle des services et la compilation/recette du fork Linux restent à réaliser. macOS reste explicitement non pris en charge pour le mode permanent.

## Correctifs

- Codes d'installation aléatoires renforcés, valables **15 minutes**, stockés sous forme d'empreinte SHA-256. Affichage uniquement lors de leur création.
- Enrôlement avec preuve de possession Ed25519. Association atomique à la vraie identité du poste ; une autre clé ou un autre ID ne peuvent pas remplacer cette association. Une réponse perdue peut être redemandée par la même identité pendant la validité du code, avec une nouvelle preuve signée.
- Signature des heartbeats, nonce à usage unique persistant en base, horodatage borné et contrôle de la licence. Refus des requêtes anonymes, rejouées ou modifiées et des postes supprimés. Comparaison chronologique des expirations, y compris si la date enregistrée comporte un autre fuseau horaire.
- Émission et renouvellement de jetons réseau compatibles avec le fork, rôle Viewer et rattachement à la licence. Leur expiration est limitée à cinq minutes ou à l'échéance de la licence si elle arrive avant.
- Suppression d'une fiche : aucun nouveau renouvellement possible. Un agent joignable détecte le refus au prochain renouvellement et arrête RustDesk ; une autorisation déjà délivrée expire au plus tard à son échéance. Le fork contrôle les sessions ouvertes par son canal d'autorisation périodique : la fermeture effective intervient au contrôle suivant, pas nécessairement à la milliseconde de la suppression.
- Service Windows d'autorisation `RelaisDeskFleet`, indépendant de la fenêtre Viewer, à démarrage automatique et avec récupération après échec. Identité et configuration sous `%ProgramData%\RelaisDeskFleet`, permissions Windows restreintes ; pas de clé dans une URI ou dans les arguments du service.
- Un même parcours d'installation pour GUI et `--enroll` : plus d'ID `AUTO-ID` / `AUTO-PENDING`, ni de réussite annoncée après un simple enregistrement en base. Vérifications avant enrôlement et attente de la première autorisation/service actif avant succès. Le mot de passe permanent n'est jamais envoyé à l'API.
- Suppression locale explicite par `--unenroll`, également appelée par les désinstalleurs Viewer et combiné. Les clés d'identité et le mot de passe RustDesk lui-même ne sont pas supprimés : seules les données propres au nouvel agent sont retirées.
- Bouton web relié au protocole installé `relaisdesk://connect/DEV-...`. Le configurateur vérifie l'URI, se connecte avec sa session technicien, vérifie l'appartenance du poste, demande confirmation, puis recontrôle la cible avant ouverture. Le bouton transmet un identifiant de fiche, jamais un secret.
- Correction des formats JSON attendus par le configurateur pour la liste et la création des postes. L'ancienne liste attendait un tableau nu et la création une réponse plate, alors que l'API renvoyait des enveloppes.
- Pagination API, validation de la longueur des alias/notes, rafraîchissement natif moins fréquent et absence de rafraîchissement du parc quand son onglet est masqué. Le navigateur charge les pages suivantes sur demande et indique que ses compteurs/recherche portent sur les postes chargés.
- Adresse d'origine extraite derrière le Nginx local ; refus des en-têtes de proxy fournis par une connexion non locale. Le champ Cloudflare n'est pas pris comme autorité dans ce nouveau parcours.
- Le statut affiché décrit désormais l'agent et le service actifs, **pas une garantie que le bureau distant est joignable**. Les états d'installation remplacent l'exposition des codes dans la liste.
- Correction macOS/Darwin et confinement du défilement du tableau sur mobile. Nettoyage des données de parc et du code affiché à la déconnexion du portail.

Les installations de postes ne sont pas limitées commercialement. La pagination n'est pas une limite de licence. Les autorisations conservent le tenant, le rôle et la capacité de l'offre ; la simultanéité doit aussi être validée dans la recette réelle.

## Compatibilité et migration future — aucune action serveur effectuée

La migration locale ajoute `devices.enrollment_version` et une table de nonces avec suppression en cascade. Les anciennes fiches reçoivent la version 0 : leur association historique, qui n'était pas authentifiée, n'est pas automatiquement considérée fiable. Elles apparaîtront « Réinstallation requise » et devront être supprimées puis recréées pour obtenir un nouveau code. Les autres données de compte, licences et factures ne sont pas supprimées.

Les anciens Viewers ne savent pas signer ces requêtes ; les nouveaux endpoints refusent leurs tentatives de parc non authentifiées. **Ne pas publier séparément la nouvelle console, l'API ou les launchers sans prévoir leur mise à niveau coordonnée**, après les essais. La téléassistance temporaire utilise toujours ses endpoints existants.

## Préparer la recette Windows, sans utiliser Oracle

1. Préparer une API de test séparée avec une base temporaire, des clés de test et un hbbs/hbbr du fork. Ne pas employer les secrets ou la base de production.
2. Compiler les launchers avec cette URL de test et les empreintes des artefacts du fork. Le paramètre additionnel `-RustDeskForkWindowsServicePath` du script de build permet d'épingler l'exécutable **natif installé en service** lorsqu'il diffère du wrapper portable intégré. Ne pas remplacer l'empreinte par celle d'un RustDesk communautaire quelconque.
3. Attention : le script historique `installer/build.ps1` copie aussi ses résultats vers les téléchargements locaux et réalise d'autres emballages. Il n'a pas été exécuté pendant ces corrections. Pour une recette isolée, compiler dans un dossier de test et ne pas transférer ses sorties sur OVH.
4. Sur le PC distant de test : installer le fork RustDesk en service `RustDesk`, dans `Program Files`. Le Viewer vérifie son chemin résolu et son empreinte ; un service installé ailleurs ou avec une empreinte différente est refusé.
5. Configurer un mot de passe permanent fort dans les paramètres de sécurité du service RustDesk. Ce paramètre n'est pas créé ou affaibli automatiquement par l'enrôlement. Vérifier que l'identité et le mot de passe du service sont présents dans sa configuration LocalService.
6. Générer un nouveau code dans la console de test, puis lancer le Viewer **en administrateur**, soit en saisissant le code dans sa fenêtre, soit avec `viewer.exe --enroll PERM-...` depuis son répertoire d'installation. Pour le portable, utiliser son vrai nom/chemin, par exemple `RelaisDesk_Portable.exe --enroll PERM-...`.
7. La confirmation d'installation n'est affichée qu'après la première autorisation réussie et le démarrage du service RustDesk. Si un message indique « service enregistré mais connexion non confirmée », ne pas considérer le poste comme prêt. Une installation interrompue se retire explicitement avec `--unenroll` avant de recommencer avec un nouveau code.
8. Sur le PC technicien, installer le configurateur de test pour enregistrer le protocole URL. La version portable seule n'enregistre pas ce protocole. Se connecter avec la licence de test propriétaire du poste ; vérifier le bouton web et la demande de confirmation. RustDesk peut demander le mot de passe permanent du poste.

### Cas à valider réellement

- Installation propre, ID identique à celui du service, prise en main et transfert autorisés.
- Fermeture du Viewer, redémarrage Windows, accès avant ouverture de session si cette fonction est attendue du service RustDesk utilisé.
- Connexion via le site et via le configurateur, y compris avec le configurateur déjà ouvert.
- Deux postes et plusieurs techniciens : limite de simultanéité exacte de l'offre, aucune limite sur les installations inactives.
- Licence expirée/révoquée, suppression du poste, arrêt du service d'autorisation, panne d'API au-delà de l'expiration du jeton : interruption de l'accès, y compris d'une session déjà ouverte.
- Tentative de réutiliser le code avec un autre poste ou une autre clé : refus.
- Compte/licence d'un autre client : refus de consultation/connexion à la fiche.
- Désinstallation : service d'autorisation arrêté, données propres à l'agent retirées, aucun accès permanent restant via un ancien jeton.

## Vérifications réalisées

- Tests Go complets réussis dans `database`, `api`, `installer/viewer`, `installer/configurator` et `tests`.
- `go vet` réussi dans ces cinq modules.
- Nouvelles non-régressions : signature de tous les champs, rejet des identités factices, des codes expirés/historiques, du rejeu, de l'usurpation d'identité, des licences expirées et des postes supprimés ; enrôlement concurrent ; confidentialité du code en base ; format JSON/pagination ; analyse stricte des URI et refus des erreurs HTTP par le Viewer.
- Tests Edge locaux avec chaque requête interceptée : création/renommage/suppression simulés, recherche, sept langues, alias HTML conservé comme texte, macOS identifié correctement. Aucun appel externe transmis. Largeur mobile : **390 px pour une fenêtre de 390 px**, contre 961 px avant correction.
- Vérification syntaxique des deux fichiers JavaScript et du script PowerShell.
- Aucun service ni désinstalleur exécuté. Les installateurs NSIS et les recettes Windows/Linux réelles restent à valider. Les tests spécifiques Linux ont été compilés, pas exécutés sur ce PC Windows ; macOS permanent reste non disponible.
- Le complément Linux modifie maintenant le fork client : `src/relaisdesk_auth.rs` et `src/relaisdesk_fleet_linux.rs`. Ses sources exactes devront être publiées avant distribution d'un nouveau binaire. Le fork serveur est inchangé ; aucune connexion à Oracle ni publication GitHub n'a eu lieu.

Captures et résultat navigateur : [preuves locales](C:/Users/Administrator/Documents/Projets/projet/output/tests/fleet-fixed-2026-09-10). Test navigateur relançable : [fleet-browser.cjs](C:/Users/Administrator/Documents/Projets/projet/tests/fleet-browser.cjs).

## Fichiers du site à transférer ultérieurement sur OVH

- `relaisdesk/client/app.js`
- `relaisdesk/client/index.html`
- `relaisdesk/client/styles.css`
- `relaisdesk/i18n.js`

Ces fichiers ont été modifiés directement dans le dossier `relaisdesk/`. Aucun fichier API, secret ou service Oracle n'y a été ajouté. Attendre la validation et la décision de déploiement coordonné avant transfert.
