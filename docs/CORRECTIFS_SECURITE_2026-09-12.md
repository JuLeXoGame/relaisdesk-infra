# Correctifs de sécurité du 12 septembre 2026

État initial : corrections dans les sources locales et tests automatisés validés. Puis, sur demande distincte de l'exploitant, **les correctifs API et Nginx ont été déployés sur Oracle le 12 septembre 2026** : voir le [compte rendu de déploiement](rapports/DEPLOIEMENT_SECURITE_ORACLE_2026-09-12.md). Les pages client OVH correspondantes étaient déjà publiées et ont été vérifiées. Aucun envoi GitHub ni remplacement des exécutables distribués n'a été effectué.

## Les neuf points corrigés

| Point constaté | Correction |
| --- | --- |
| Le service permanent Windows pouvait utiliser un exécutable ou des DLL issus du cache utilisateur. | Le Viewer intègre les trois composants natifs du build. L'exécutable doit correspondre à son SHA-256 injecté ; les fichiers installés sont comparés aux octets intégrés, DLL comprises. Aucun repli vers le cache. |
| Une adresse `CF-Connecting-IP` fournie par le client pouvait contourner la limitation des tentatives. | L'API ne lit que `X-Real-IP`, uniquement lorsque la connexion provient de la boucle locale. Le modèle Nginx écrase cet en-tête et efface `CF-Connecting-IP`. |
| La connexion administrateur par licence pouvait éviter la 2FA. | Les connexions par licence et par e-mail passent par la même vérification des facteurs. |
| Les liens de connexion et de réinitialisation pouvaient éviter une 2FA existante. | Le facteur existant ou un code de secours est obligatoire ; le lien e-mail seul ne suffit plus. Les tentatives sont limitées par lien. |
| Certaines sessions administrateur survivaient à la révocation de leur licence. | Les administrateurs de compte ont des sessions liées à une licence, avec contrôle de son statut, de son échéance et du rôle courant. Les sessions issues du secret maître restent distinctes. |
| Un facteur existant pouvait être remplacé par un nouvel enrôlement. | Préparation d'enrôlement conservée côté serveur, expirant après dix minutes ; refus du remplacement direct. Activation authentifiée avec mot de passe ; désactivation et renouvellement des codes avec mot de passe et facteur existant. |
| Le Technicien conservait des identifiants secrets en clair. | Stockage Windows chiffré avec DPAPI lié à l'utilisateur, et migration du fichier existant à sa lecture. Sous Linux/macOS, seuls les identifiants non secrets sont mémorisés. |
| Un changement de mot de passe ne révoquait pas les anciennes sessions. | Révocation transactionnelle des sessions client/technicien concernées, challenges et liens e-mail. Même protection lors de l'activation/désactivation de la 2FA. |
| Plusieurs requêtes simultanées pouvaient consommer le même challenge ou code de secours. | Vérification, consommation et émission de session dans une même transaction SQLite. Protection supplémentaire contre le rejeu d'un même code TOTP. |

Compléments liés : déconnexion administrateur invalidant aussi la session liée à la licence ; contrôle transactionnel des droits et identifiants lors de la connexion ; création initiale du mot de passe administrateur ne permettant plus de réinitialiser un compte existant ; correction du nom de table de licences dans le repli administrateur du middleware technicien.

## Conséquences à prévoir lors du futur déploiement

- Au premier démarrage de cette API, la migration `20260912-auth-v1` invalide **une seule fois** les anciennes sessions client, technicien et administrateur ainsi que les liens/challenges de connexion en cours. Il faudra se reconnecter ou demander un nouveau lien. Les licences, appareils, commandes, factures et facteurs 2FA ne sont pas supprimés.
- Le changement d'un mot de passe ou d'un facteur peut déconnecter les sessions technicien des licences du même compte client : les sessions actuelles sont liées à une licence, pas à une identité individuelle de collaborateur.
- Un compte protégé par 2FA doit conserver ses codes de secours hors de l'application. La récupération par e-mail ne désactive plus cette protection. Aucun contournement automatique n'a été ajouté en cas de perte des deux moyens.
- Un TOTP déjà utilisé peut être refusé jusqu'au prochain code ; un code de secours reste utilisable une seule fois.
- Sous Windows, DPAPI protège le stockage au repos mais ne protège pas contre un logiciel malveillant s'exécutant avec les droits du même utilisateur. Les anciennes copies/sauvegardes du fichier en clair ne sont pas effacées par cette migration.
- Sous Linux/macOS, le Technicien demandera à nouveau le mot de passe ou la clé de licence au démarrage. L'intégration d'un coffre système n'est pas implémentée ; aucun repli en clair n'est autorisé.

## Service permanent et compilation Windows

Le service natif est installé dans `Program Files\RelaisDeskEngine`, avec un dossier protégé. Une installation indépendante portant déjà le nom de service `RustDesk` n'est pas remplacée automatiquement. La désinstallation ne supprime pas cette installation indépendante et ne termine que les processus dont le chemin correspond au moteur RelaisDesk. Elle préserve les fichiers inconnus.

Une ancienne installation du moteur dans `Program Files\RustDesk` demande donc une migration manuelle contrôlée : identifier et sauvegarder sa configuration, arrêter/désinstaller uniquement le service confirmé comme ancien moteur RelaisDesk, puis réinstaller avec le nouveau Viewer. Ne pas supprimer une installation RustDesk tierce pour débloquer l'opération sans l'accord du propriétaire.

Les builds du Viewer nécessitent désormais, dans le même dossier de compilation du fork :

- `rustdesk.exe` **natif**, pas son wrapper portable ;
- `sciter.dll` ;
- `dylib_virtual_display.dll`.

`scripts/prepare-fleet-payload.ps1` vérifie leur présence/format, refuse les fichiers ou liens inattendus, puis prépare `installer/viewer/embedded/fleet/`. La provenance du build reste à contrôler : une signature `MZ` n'est pas une preuve d'authenticité.

Paramètres à fournir lors de la prochaine compilation : `-RustDeskForkWindowsServicePath` pour `installer/build.ps1`, ou `-WindowsNativeExecutable` pour `scripts/build-security-release.ps1`. `scripts/build-rustdesk-windows.ps1` récupère les composants de son propre build natif. Le SHA-256 natif est injecté dans le Viewer. Sans ces composants ou cette empreinte, l'installation permanente est refusée, même si un cache utilisateur contient un exécutable.

Les exécutables de `relaisdesk/downloads/` n'ont pas été reconstruits et ne bénéficient donc pas encore de ces changements de sources.

## Fichiers web et séquence recommandée

Les seules modifications web de cette intervention sont :

- `relaisdesk/client/index.html` ;
- `relaisdesk/client/app.js`.

Ces fichiers ajoutent la saisie du facteur existant à la récupération du mot de passe, les confirmations de sécurité et la reconnexion après révocation de session. Leur version de cache est `20260912-security`.

Pour une future publication autorisée : sauvegarder la base, préparer/tester l'API et les nouveaux programmes, transférer ces deux fichiers sur OVH puis basculer l'API pendant une courte fenêtre coordonnée. Le site reste chez OVH. Le modèle `scripts/nginx-api-relaisdesk.conf.example` concerne uniquement Oracle et ne doit pas être placé dans le site. Vérifier que l'API écoute sur la boucle locale et que Nginx fixe `X-Real-IP` à partir de l'adresse réseau réelle.

## Vérifications réalisées et limites

- Suites Go complètes de `api/` et `database/`.
- Suites Go Windows du Viewer et du Technicien avec le mode graphique de test `-tags ci` ; tests DPAPI exécutés.
- Analyse statique `go vet` sur ces quatre modules.
- Tests de non-régression des neuf scénarios, de la migration unique, de la déconnexion administrateur et d'un identifiant de licence devenu obsolète pendant une connexion.
- Dix exécutions du test concurrent : seize tentatives avec le même code de secours, une seule session obtenue à chaque exécution.
- Tests du paquet natif avec fichiers inertes : cache utilisateur ignoré, DLL altérée refusée, DLL inconnue préservée et installation bloquée, commande d'un service indépendant rejetée.
- Vérification syntaxique des quatre scripts PowerShell modifiés et du JavaScript client.
- Test du client web dans un navigateur automatisé avec API simulée : récupération protégée, activation/désactivation 2FA, champs transmis et déconnexion après changement ; aucune erreur JavaScript.
- Compilation croisée de l'API Linux et des tests des deux programmes Linux ; pas d'exécution Linux sur cette machine Windows.

Restent nécessaires avant diffusion : essais des nouveaux exécutables réels dans une VM Windows jetable (installation, mise à jour, mot de passe permanent, accès distant, désinstallation et coexistence avec un RustDesk tiers), ainsi qu'une recette Linux réelle. Le test Windows installant effectivement un service est désormais désactivé par défaut, même dans un terminal administrateur ; il demande un opt-in explicite et refuse une machine ayant déjà un service RustDesk/Fleet. Il n'a pas été exécuté ici.

Ces résultats couvrent les corrections identifiées ; ils ne constituent ni une garantie d'absence de toute vulnérabilité ni une vérification du serveur actuellement en production.
