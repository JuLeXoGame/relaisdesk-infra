# Audit local de la console de gestion de parc — 10 septembre 2026

> Ce document décrit l'état constaté avant correction. Voir [les corrections et leurs limites de validation](C:/Users/Administrator/Documents/Projets/projet/docs/audits/CORRECTIONS_PARC_2026-09-10.md) pour la suite. Les sondes historiques dans le dossier d'audit ne sont pas des tests de non-régression de la version corrigée.

## Conclusion

La console constitue une base utile d'inventaire, mais **la version locale examinée ne peut pas être validée comme un accès permanent sans surveillance fonctionnel et sécurisé**. Les écrans et opérations de gestion sont présents ; leur raccordement au mécanisme d'autorisation du fork est incomplet.

Audit demandé en lecture/analyse : aucun fichier applicatif corrigé, aucun binaire publié, aucune connexion ni intervention sur Oracle. Seuls ce rapport et des fichiers de tests/constats isolés ont été ajoutés localement.

P1 = à traiter avant utilisation réelle de cette fonctionnalité. P2 = anomalie à corriger, sans preuve ici d'une compromission du contrôle distant.

## 1. [P1] Un code d'enrôlement déjà utilisé peut remplacer l'identité du poste

Source : [database/devices.go](C:/Users/Administrator/Documents/Projets/projet/database/devices.go:158), particulièrement l'UPDATE à la ligne 210.

L'API retrouve le poste avec `permanent_code`, puis remplace sans condition son ID RustDesk et sa clé publique. Elle ne vérifie ni la possession de la clé déjà enregistrée, ni l'identité précédente. Le code reste réutilisable et visible dans la console après l'enrôlement.

Reproduction locale : deux appels successifs avec le même code mais deux ID/clefs différents retournent HTTP 200. La seconde identité remplace la première. Des chaînes qui ne sont pas des clés Ed25519 valides sont également acceptées.

Conséquence : quelqu'un qui a conservé ce code peut réaffecter la fiche de parc à un autre poste. Le bouton du technicien utilisera alors l'ID substitué. Ce test ne démontre pas une prise de contrôle arbitraire du PC d'origine ni un contournement des signatures du fork.

Correction attendue : code d'installation temporaire, consommé atomiquement ; identité cryptographique attachée au poste ; renouvellement signé ; réenrôlement explicitement autorisé. L'accès peut être permanent sans que le code d'installation soit un secret réutilisable indéfiniment.

## 2. [P1] L'accès permanent n'est pas raccordé de bout en bout

### Autorisation réseau manquante

Sources : [parcours permanent du Viewer](C:/Users/Administrator/Documents/Projets/projet/installer/viewer/gui.go:269), [réponse d'enrôlement API](C:/Users/Administrator/Documents/Projets/projet/api/handlers/devices.go:22), [endpoint de jeton existant](C:/Users/Administrator/Documents/Projets/projet/api/handlers/network_tokens.go:80).

`ensureViewerNetworkIdentity()` crée/charge une clé et calcule le chemin du jeton ; cela n'émet pas de jeton. Le parcours permanent ne demande ni n'enregistre de jeton réseau signé et ne lance pas son renouvellement. La réponse d'enrôlement n'en contient pas. L'endpoint Viewer existant valide les codes temporaires, pas les entrées de la nouvelle table `devices`.

Reproduction locale : un code PERM valide pour l'enrôlement reçoit HTTP 401 à l'endpoint de jeton Viewer, avec une clé publique de format valide. Le [client du fork](C:/Users/Administrator/Documents/Projets/projet/rustdesk/src/relaisdesk_auth.rs:40) lit effectivement le fichier de jeton pour produire sa preuve. Sur une nouvelle installation, ce chemin ne donne donc pas l'autorisation requise ; un ancien jeton éventuellement présent ne constitue pas une solution pérenne.

### RustDesk arrêté et non relancé après configuration

Sources : [configuration Windows](C:/Users/Administrator/Documents/Projets/projet/installer/viewer/rustdesk_windows.go:189), [suite du parcours permanent](C:/Users/Administrator/Documents/Projets/projet/installer/viewer/gui.go:310).

Le Viewer lance d'abord RustDesk pour récupérer son ID. La configuration tue ensuite RustDesk. Contrairement au parcours temporaire, le parcours permanent ne le relance pas après cette étape. Le message de succès ne prouve donc pas que le poste est joignable.

Si aucun ID n'est trouvé, `AUTO-PENDING` est enregistré au lieu de faire échouer/reprendre proprement l'opération. Il n'existe pas de mise à jour ultérieure de cet ID dans les heartbeats.

### Fermeture et redémarrage non pris en charge

Sources : [fermeture commune du Viewer](C:/Users/Administrator/Documents/Projets/projet/installer/viewer/gui.go:47), [message affiché](C:/Users/Administrator/Documents/Projets/projet/installer/viewer/i18n.go:182).

Le message dit que l'accès reste actif après fermeture. Mais le gestionnaire de fermeture supprime la configuration et arrête RustDesk. Le heartbeat permanent n'est qu'une goroutine du Viewer, arrêtée à la sortie du processus. Le nouveau parcours ne met pas en place de service persistant reprenant l'enrôlement et le renouvellement des autorisations au redémarrage.

### Commande CLI incomplète

Source : [installer/viewer/main.go](C:/Users/Administrator/Documents/Projets/projet/installer/viewer/main.go:26).

La commande `--enroll`, proposée par les deux consoles, envoie littéralement `AUTO-ID` et une clé publique vide, imprime « Succès », puis quitte. Elle n'installe/configure pas le poste et ne démarre pas de service ni de heartbeat.

Reproduction avec transport HTTP entièrement simulé : un seul appel d'enrôlement, identité factice, puis retour du programme. Aucun lancement de RustDesk n'a été effectué pendant le test.

Sur Linux, le parcours normal reste celui des codes temporaires ; il ne route pas les codes PERM vers une implémentation persistante. Le choix `use-permanent-password` dans la configuration ne suffit pas à créer un accès permanent ni une autorisation réseau.

Correction attendue : un même parcours complet GUI/CLI, vraie identité du poste, émission/renouvellement de jetons compatibles avec le fork, démarrage effectif et persistant, gestion explicite de l'autorisation de contrôle sans présence locale, erreurs traitées avant d'afficher un succès. Ne pas désactiver l'authentification du fork pour masquer ce problème.

## 3. [P1] Le bouton de connexion web n'a pas de gestionnaire livré correspondant

Source : [relaisdesk/client/app.js](C:/Users/Administrator/Documents/Projets/projet/relaisdesk/client/app.js:708).

Le bouton ouvre `relaisdesk://connect/<id>`. Les installateurs RelaisDesk examinés n'enregistrent pas ce protocole et le [point d'entrée du configurateur](C:/Users/Administrator/Documents/Projets/projet/installer/configurator/main.go:8) ne traite aucun argument de connexion.

Le fork RustDesk possède son mécanisme de protocole lié au nom de son application ; cela ne constitue pas une implémentation du nouveau lien RelaisDesk vers le configurateur. Le simple ajout du bouton web n'effectue pas ce raccordement.

Correction attendue : handler installé/désinstallé proprement, validation stricte de l'URI et du poste, session technicien et autorisation réseau valides, comportement clair si le logiciel est absent. Ne pas placer de secret de connexion dans l'URL.

Aucune URI native n'a été exécutée sur ce PC pendant l'audit.

## 4. [P1/P2] Présence usurpable et révocation réseau non implémentée

Sources : [heartbeat public](C:/Users/Administrator/Documents/Projets/projet/api/handlers/devices.go:101), [mise à jour en base](C:/Users/Administrator/Documents/Projets/projet/database/devices.go:231), [suppression](C:/Users/Administrator/Documents/Projets/projet/database/devices.go:439), [client heartbeat](C:/Users/Administrator/Documents/Projets/projet/installer/viewer/api.go:255).

- Le heartbeat accepte le simple ID `DEV-...` sans preuve/signature d'appareil. Il ne vérifie pas l'état de la licence. Reproduction : un poste non encore enrôlé, rattaché à une licence expirée, passe « online » via une requête anonyme. Cela falsifie l'inventaire, sans démontrer un accès distant.
- « Online » signifie seulement que le processus Viewer a envoyé un heartbeat récent, pas que RustDesk est connecté au réseau et joignable. Le parcours actuel peut précisément arrêter RustDesk puis continuer les heartbeats.
- La suppression invalide bien la fiche, son code d'enrôlement et les heartbeats suivants côté API. Mais elle ne révoque aucune autorisation réseau et ne commande pas l'arrêt de l'agent. Le Viewer ignore le statut HTTP et considère même un 404 comme un succès : reproduit avec un transport simulé.

La [confirmation de suppression](C:/Users/Administrator/Documents/Projets/projet/relaisdesk/i18n.js:274) promet une révocation d'accès permanent que ce code ne met pas en œuvre. Les défauts du point 2 empêchent déjà le fonctionnement normal : nous ne prétendons donc pas avoir démontré une session Oracle qui survivrait à une suppression.

Correction attendue : authentifier les heartbeats ; distinguer agent présent et RustDesk joignable ; rattacher les autorisations renouvelables au poste actif et à sa licence ; arrêt/expiration bornée des autorisations après suppression, licence expirée ou révoquée ; traiter les erreurs côté agent. Tester aussi les sessions déjà ouvertes.

## 5. [P2] IP du poste incorrecte derrière Nginx

Source : [api/handlers/devices.go](C:/Users/Administrator/Documents/Projets/projet/api/handlers/devices.go:49).

Le nouvel utilitaire ne lit que `RemoteAddr`. Avec Nginx connecté à l'API sur loopback, il stocke `127.0.0.1` au lieu de l'adresse du poste.

Reproduction : requête simulée depuis `127.0.0.1`, avec `X-Real-IP` et `X-Forwarded-For` contenant une IP de test ; la base conserve `127.0.0.1`.

Correction attendue : extraction centralisée des IP et confiance limitée aux proxys réellement autorisés, sans accepter aveuglément les en-têtes d'un appelant direct.

## 6. [P2] Console débordante sur téléphone

Sources : [styles de la console](C:/Users/Administrator/Documents/Projets/projet/relaisdesk/client/styles.css:3), [panneau parc](C:/Users/Administrator/Documents/Projets/projet/relaisdesk/client/index.html:248).

Sur une fenêtre de 390 px, la largeur du document mesurée est de **961 px**. Les cartes, le tableau et le bouton d'enrôlement débordent ; ce n'est pas uniquement un tableau défilant dans son conteneur.

Voir la [capture mobile](C:/Users/Administrator/Documents/Projets/projet/output/tests/audit-parc-2026-09-10/console-mobile.png). Corriger les contraintes de taille des grilles/enfants et confiner le défilement horizontal au tableau, puis retester les boutons et le formulaire sur petit écran.

## 7. [P2] macOS identifié comme Windows dans le tableau web

Source : [relaisdesk/client/app.js](C:/Users/Administrator/Documents/Projets/projet/relaisdesk/client/app.js:658).

Le test `includes('win')` précède celui de `darwin`. Puisque `darwin` contient `win`, un Viewer macOS est affiché Windows. Reproduit dans le navigateur avec `os: "darwin"`.

Correction attendue : correspondance explicite des valeurs d'OS, ou traitement de Darwin avant Windows.

## Qualité et légèreté de l'API

Les index de la table `devices` sont présents. En revanche, le [configurateur](C:/Users/Administrator/Documents/Projets/projet/installer/configurator/gui.go:488) relit et reconstruit toute la liste toutes les quatre secondes, même si l'utilisateur consulte l'autre onglet. Les listes API ne sont pas paginées ; alias/notes n'ont pas de bornes métier côté serveur, même si le formulaire web en possède.

À améliorer avant un parc important : pagination, rafraîchissement limité à l'onglet visible, rafraîchissement différentiel/adaptatif, validation des tailles côté API. Ce constat n'est pas un benchmark de charge du serveur.

Cela ne nécessite pas de limiter le nombre de postes installés. La règle commerciale reste la limitation des techniciens simultanés selon l'offre ; sa validation complète avec le nouveau mode permanent devra faire partie de la recette après raccordement des jetons.

## Vérifications effectuées et limites

- `go test -count=1 -trimpath -mod=readonly ./...` : succès dans `api`, `database`, `installer/viewer` et `installer/configurator`, sous Windows.
- `go vet -mod=readonly ./...` : succès dans ces quatre modules.
- Sept sondes supplémentaires isolées : réenrôlement, heartbeat sans authentification/licence expirée, refus de jeton PERM, IP derrière proxy, isolation entre licences de techniciens, CLI factice, erreur HTTP de heartbeat ignorée. Six reproduisent les comportements indésirables ; le contrôle d'isolation réussit. Leur statut PASS signifie que le comportement décrit a été observé, **pas que le défaut est corrigé**.
- Navigateur Edge isolé : affichage, recherche, création simulée, renommage, suppression, sept langues ; aucune erreur JavaScript. Une valeur d'alias contenant du HTML reste du texte, sans élément HTML injecté.
- Chaque requête du navigateur est satisfaite localement avec les fichiers du site ou une API simulée ; aucun appel externe transmis. Les contrôles Go utilisent des bases temporaires, httptest ou un transport simulé.
- Les accès de liste/suppression d'un technicien à une autre licence sont refusés. Les tests existants de cycle client/technicien passent. Ce n'est pas une preuve exhaustive de l'absence de toute faille inter-comptes.
- Les forks locaux `rustdesk` et `rustdesk-server` sont restés sans modification Git.
- Pas de recette réelle entre deux PC, de redémarrage de machine, de session sans utilisateur connecté, de compilation/essai Linux/macOS, ni de contrôle de la version effectivement déployée sur Oracle. Aucun réglage de production n'a été lu ou modifié.

Les sources de sondes, overlays Go, résultats navigateur et captures sont dans [le dossier de preuves local](C:/Users/Administrator/Documents/Projets/projet/output/tests/audit-parc-2026-09-10).

## Suite recommandée

Corriger localement les points P1, puis transformer les sondes en véritables tests de non-régression exigeant le refus des comportements dangereux. Valider ensuite sur deux machines de test : installation propre, vraie prise en main, fermeture du Viewer, redémarrage, expiration du jeton, suspension de licence, suppression du poste, réutilisation d'un code et limites de simultanéité. Aucun déploiement ne fait partie de cet audit.
