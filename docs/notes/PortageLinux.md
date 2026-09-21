Plan d’implémentation complet : sécurisation et portage Linux / multiplateforme du configurateur

Ce document décrit la marche à suivre pour transformer l’installateur configurateur en une application compatible Windows et Linux, avec une interface graphique unique, une logique système séparée, une installation propre et des règles de sécurité strictes.

1\. Objectif du portage

Le configurateur doit devenir une application multiplateforme capable de :



&#x20;   fonctionner sur Windows et Linux ;

&#x20;   conserver une interface graphique commune ;

&#x20;   séparer clairement la logique spécifique à chaque système ;

&#x20;   télécharger RustDesk de manière sécurisée ;

&#x20;   vérifier l’intégrité du fichier téléchargé avant installation ;

&#x20;   installer RustDesk proprement selon l’OS ;

&#x20;   configurer RustDesk correctement ;

&#x20;   nettoyer tous les fichiers temporaires ;

&#x20;   respecter les standards Linux en matière d’installation, de privilèges et d’intégration au système.



2\. Principes directeurs

2.1 Interface graphique unique

L’interface graphique doit être écrite une seule fois pour toutes les plateformes.

Elle doit être indépendante du système d’exploitation.

Elle ne doit contenir aucune logique système directe.

Elle doit seulement appeler des fonctions de haut niveau comme :



&#x20;   télécharger RustDesk ;

&#x20;   vérifier RustDesk ;

&#x20;   installer RustDesk ;

&#x20;   configurer RustDesk ;

&#x20;   nettoyer les fichiers temporaires.



2.2 Séparation de la logique système

Tout ce qui dépend du système d’exploitation doit être isolé dans des fichiers séparés.

La séparation doit être faite avec les build tags Go.

Les fichiers Windows ne doivent jamais être compilés sous Linux.

Les fichiers Linux ne doivent jamais être compilés sous Windows.

2.3 Sécurité

Les règles suivantes sont obligatoires :



&#x20;   aucun fichier téléchargé ne doit être exécuté ou installé avant vérification SHA-256 ;

&#x20;   aucun secret ne doit apparaître dans les logs ;

&#x20;   aucun fichier temporaire ne doit rester sur le disque après l’installation ;

&#x20;   l’interface graphique ne doit jamais être lancée en root sous Linux ;

&#x20;   les commandes système doivent être exécutées avec des arguments séparés ;

&#x20;   il est interdit de passer par un shell non maîtrisé ;

&#x20;   les téléchargements doivent se faire uniquement en HTTPS ;

&#x20;   les redirections HTTP non contrôlées doivent être refusées.



3\. Framework graphique recommandé

Le framework graphique actuel, spécifique à Windows, doit être remplacé par Fyne.

Fyne est recommandé car il permet de créer une interface graphique Go multiplateforme.

L’interface graphique doit être réécrite dans le fichier gui.go.

Le fichier gui.go doit être commun à Windows et Linux.

4\. Organisation des fichiers

4.1 Fichiers à modifier

installer/configurator/gui.go

Ce fichier doit être réécrit avec Fyne.

Il doit contenir uniquement l’interface graphique.

Il ne doit contenir aucun code spécifique à Windows ou Linux.

installer/configurator/config.go

Ce fichier doit contenir les éléments partagés :



&#x20;   structures communes ;

&#x20;   interfaces ;

&#x20;   types communs ;

&#x20;   fonctions utilitaires non dépendantes de l’OS ;

&#x20;   éventuellement les constantes communes non sensibles.



4.2 Fichier à supprimer

installer/configurator/rustdesk.go

Ce fichier doit être supprimé car il doit être remplacé par des fichiers spécifiques à chaque OS.

4.3 Fichiers Windows à créer

installer/configurator/rustdesk\_windows.go

Ce fichier doit contenir toute la logique Windows :



&#x20;   téléchargement de l’exécutable RustDesk ;

&#x20;   vérification SHA-256 ;

&#x20;   installation ;

&#x20;   configuration ;

&#x20;   création de raccourcis si nécessaire ;

&#x20;   nettoyage.



installer/configurator/config\_windows.go

Ce fichier doit contenir les constantes Windows :



&#x20;   URL de téléchargement du fichier RustDesk pour Windows ;

&#x20;   hash SHA-256 attendu ;

&#x20;   chemins de configuration ;

&#x20;   noms de fichiers ;

&#x20;   éventuels paramètres spécifiques Windows.



La première ligne du fichier doit être :

//go:build windows

4.4 Fichiers Linux à créer

installer/configurator/rustdesk\_linux.go

Ce fichier doit contenir toute la logique Linux :



&#x20;   téléchargement du paquet .deb ;

&#x20;   vérification SHA-256 ;

&#x20;   installation via pkexec ;

&#x20;   configuration ;

&#x20;   nettoyage.



installer/configurator/config\_linux.go

Ce fichier doit contenir les constantes Linux :



&#x20;   URL de téléchargement du paquet .deb ;

&#x20;   hash SHA-256 attendu ;

&#x20;   chemins de configuration ;

&#x20;   nom du paquet ;

&#x20;   éventuels paramètres spécifiques Linux.



La première ligne du fichier doit être :

//go:build linux

4.5 Fichier d’intégration Linux à créer

installer/configurator/desktop/configurator.desktop

Ce fichier doit permettre l’intégration du configurateur dans le menu des applications Linux.

Il doit contenir au minimum :



&#x20;   le nom de l’application ;

&#x20;   la commande d’exécution ;

&#x20;   l’icône ;

&#x20;   le type Application ;

&#x20;   Terminal=false ;

&#x20;   une catégorie.



5\. Règles pour l’interface graphique

Le fichier gui.go doit respecter les règles suivantes.

5.1 Ce qui est autorisé dans gui.go



&#x20;   création de la fenêtre ;

&#x20;   création des boutons ;

&#x20;   affichage des messages ;

&#x20;   affichage des erreurs ;

&#x20;   affichage de la progression ;

&#x20;   appel des fonctions métier abstraites.



5.2 Ce qui est interdit dans gui.go



&#x20;   appels à apt ;

&#x20;   appels à dpkg ;

&#x20;   appels à pkexec ;

&#x20;   appels à systemctl ;

&#x20;   manipulation du registre Windows ;

&#x20;   création de raccourcis Windows ;

&#x20;   téléchargement direct ;

&#x20;   exécution de commandes système ;

&#x20;   écriture dans des chemins spécifiques à un OS ;

&#x20;   présence de secrets ou de hash.



5.3 Expérience utilisateur

L’interface doit :



&#x20;   informer l’utilisateur de l’étape en cours ;

&#x20;   afficher les erreurs de manière claire ;

&#x20;   ne pas bloquer l’interface pendant les opérations longues ;

&#x20;   permettre la fermeture propre de l’application ;

&#x20;   éviter tout dialogue inutile.



6\. Logique Windows

Le fichier rustdesk\_windows.go doit reprendre la logique existante tout en la sécurisant.

6.1 Téléchargement

L’application doit :



&#x20;   télécharger l’exécutable RustDesk pour Windows ;

&#x20;   utiliser HTTPS ;

&#x20;   refuser les redirections non contrôlées ;

&#x20;   écrire le fichier dans un dossier temporaire ;

&#x20;   ne jamais exécuter le fichier avant vérification.



6.2 Vérification

L’application doit :



&#x20;   calculer le SHA-256 du fichier téléchargé ;

&#x20;   comparer ce hash avec la valeur définie dans config\_windows.go ;

&#x20;   annuler l’installation si le hash ne correspond pas ;

&#x20;   supprimer le fichier si la vérification échoue.



6.3 Installation

L’installation doit :



&#x20;   être silencieuse si le scénario le prévoit ;

&#x20;   utiliser les privilèges nécessaires uniquement si requis ;

&#x20;   éviter toute exécution inutile ;

&#x20;   gérer les erreurs proprement.



6.4 Configuration

La configuration doit :



&#x20;   écrire dans les chemins standards Windows ;

&#x20;   ne pas écraser des données utilisateur sans nécessité ;

&#x20;   gérer les erreurs d’écriture ;

&#x20;   créer les raccourcis uniquement si cela est prévu.



6.5 Nettoyage

Après succès ou échec :



&#x20;   le fichier téléchargé doit être supprimé ;

&#x20;   le dossier temporaire doit être supprimé ;

&#x20;   aucun fichier inutile ne doit rester dans Téléchargements.



7\. Logique Linux

Le fichier rustdesk\_linux.go doit implémenter une installation propre et sécurisée.

7.1 Téléchargement du paquet

L’application doit :



&#x20;   télécharger le paquet officiel RustDesk au format .deb ;

&#x20;   utiliser HTTPS ;

&#x20;   refuser les redirections non contrôlées ;

&#x20;   créer un dossier temporaire ;

&#x20;   télécharger le .deb dans ce dossier temporaire.



7.2 Vérification du paquet

Avant toute installation, l’application doit :



&#x20;   calculer le SHA-256 du fichier .deb téléchargé ;

&#x20;   comparer ce hash avec la valeur définie dans config\_linux.go ;

&#x20;   refuser l’installation si le hash ne correspond pas ;

&#x20;   supprimer immédiatement le fichier en cas d’échec.



Cette étape est obligatoire.

Aucun paquet ne doit être installé sans vérification préalable.

7.3 Installation du paquet

L’interface graphique ne doit jamais être lancée en root.

L’installation doit être réalisée avec une élévation de privilèges ponctuelle.

La méthode recommandée est pkexec.

La commande cible doit être équivalente à :

pkexec apt install -y /chemin/temporaire/rustdesk.deb

Il est interdit de lancer cette commande à travers un shell non maîtrisé.

Il faut utiliser une exécution avec arguments séparés.

Il ne faut pas concaténer la commande dans une chaîne passée à sh ou bash.

7.4 Configuration Linux

Deux cas doivent être distingués.

Cas 1 : configuration utilisateur simple

Si RustDesk est configuré pour un usage utilisateur standard, la configuration peut être écrite dans :

\~/.config/rustdesk/

Le fichier de configuration peut être RustDesk2.toml ou équivalent selon la version utilisée.

Cas 2 : configuration service ou accès non surveillé

Si RustDesk fonctionne comme service système ou doit être contrôlé à distance sans action utilisateur, la configuration peut devoir être placée dans un chemin système, par exemple :

/etc/rustdesk/

ou

/var/lib/rustdesk/

Dans ce cas, l’écriture doit être faite avec les privilèges nécessaires.

Le code doit prévoir une séparation claire entre configuration utilisateur et configuration système.

7.5 Nettoyage Linux

Après installation, succès ou échec :



&#x20;   le fichier .deb doit être supprimé ;

&#x20;   le dossier temporaire doit être supprimé ;

&#x20;   aucun fichier temporaire ne doit rester dans /tmp ou dans Téléchargements.



Le nettoyage doit être garanti même en cas d’erreur.

8\. Sécurité obligatoire pour tous les OS

Les règles suivantes s’appliquent à Windows et Linux.

8.1 Téléchargement



&#x20;   HTTPS obligatoire ;

&#x20;   pas de téléchargement en HTTP ;

&#x20;   pas de redirection non contrôlée ;

&#x20;   URL de téléchargement définie dans la configuration ;

&#x20;   pas de construction d’URL à partir d’entrées non fiables.



8.2 Vérification



&#x20;   SHA-256 obligatoire ;

&#x20;   hash épinglé dans la configuration ;

&#x20;   vérification avant exécution ou installation ;

&#x20;   refus immédiat en cas de mismatch ;

&#x20;   suppression du fichier corrompu ou inattendu.



8.3 Exécution



&#x20;   aucune exécution avant vérification ;

&#x20;   aucune commande shell dangereuse ;

&#x20;   aucun chemin dynamique non validé ;

&#x20;   aucun secret dans les logs ;

&#x20;   aucun mot de passe dans les logs ;

&#x20;   aucun token dans les logs.



8.4 Privilèges



&#x20;   moindre privilège ;

&#x20;   interface utilisateur standard ;

&#x20;   élévation de privilèges uniquement pour l’action nécessaire ;

&#x20;   jamais d’exécution complète de l’interface en root sous Linux.



8.5 Fichiers temporaires



&#x20;   dossier temporaire sécurisé ;

&#x20;   nettoyage systématique ;

&#x20;   suppression même en cas d’erreur ;

&#x20;   aucun fichier laissé dans Téléchargements.



9\. Packaging Linux

Le configurateur lui-même doit être installé proprement sous Linux.

9.1 Mauvaise approche à éviter

Il ne faut pas fournir comme solution principale un script install.sh demandant à l’utilisateur de lancer sudo.

Cette approche n’est pas idéale car :



&#x20;   elle ne permet pas une désinstallation propre ;

&#x20;   elle ne s’intègre pas au gestionnaire de paquets ;

&#x20;   elle peut inciter à exécuter un script avec des privilèges élevés ;

&#x20;   elle est moins professionnelle qu’un paquet natif.



9.2 Approche recommandée

La solution recommandée est de générer un paquet .deb.

Le paquet doit installer :



&#x20;   le binaire du configurateur ;

&#x20;   l’icône ;

&#x20;   le fichier .desktop ;

&#x20;   éventuellement les ressources nécessaires.



Le paquet doit être créé via un outil de build sérieux.

Les outils recommandés sont :



&#x20;   Goreleaser ;

&#x20;   NFPM ;

&#x20;   ou un mécanisme équivalent capable de produire un .deb propre.



9.3 Intégration au menu des applications

Le paquet doit installer un fichier .desktop afin que le configurateur apparaisse dans le menu des applications.

Le fichier .desktop doit contenir au minimum :



&#x20;   Name ;

&#x20;   Exec ;

&#x20;   Icon ;

&#x20;   Terminal=false ;

&#x20;   Type=Application ;

&#x20;   Categories.



9.4 Alternative portable

Une alternative possible est de fournir un fichier AppImage.

Dans ce cas :



&#x20;   l’utilisateur peut lancer l’application sans installation ;

&#x20;   aucun passage par sudo n’est nécessaire ;

&#x20;   l’application reste portable ;

&#x20;   l’intégration au menu peut être optionnelle.



10\. Tests à prévoir

10.1 Tests de compilation

Le projet doit compiler pour Windows.

Le projet doit compiler pour Linux.

La compilation croisée doit être vérifiée.

10.2 Tests d’interface

L’interface doit :



&#x20;   démarrer ;

&#x20;   afficher les éléments prévus ;

&#x20;   ne pas geler pendant les opérations longues ;

&#x20;   afficher les erreurs correctement.



10.3 Tests Windows

Sous Windows, tester :



&#x20;   téléchargement ;

&#x20;   vérification SHA-256 valide ;

&#x20;   vérification SHA-256 invalide ;

&#x20;   installation ;

&#x20;   configuration ;

&#x20;   nettoyage ;

&#x20;   absence de fichiers temporaires après exécution.



10.4 Tests Linux

Sous Linux, tester :



&#x20;   téléchargement du .deb ;

&#x20;   vérification SHA-256 valide ;

&#x20;   vérification SHA-256 invalide ;

&#x20;   annulation si hash invalide ;

&#x20;   installation via pkexec ;

&#x20;   configuration utilisateur ;

&#x20;   configuration système si nécessaire ;

&#x20;   nettoyage complet ;

&#x20;   présence du raccourci dans le menu après installation du paquet ;

&#x20;   désinstallation propre du paquet.



10.5 Tests de sécurité

Tester :



&#x20;   refus d’un fichier corrompu ;

&#x20;   refus d’un fichier modifié après téléchargement ;

&#x20;   refus des redirections non contrôlées ;

&#x20;   refus d’exécution avant vérification ;

&#x20;   absence de secrets dans les logs ;

&#x20;   absence de commandes shell dangereuses ;

&#x20;   absence de fichiers temporaires résiduels.



11\. Critères d’acceptation

Le portage sera considéré comme terminé lorsque :



&#x20;   l’application compile pour Windows ;

&#x20;   l’application compile pour Linux ;

&#x20;   l’interface graphique fonctionne sur les deux OS ;

&#x20;   la logique système est bien séparée ;

&#x20;   aucun code Windows n’est présent dans les fichiers Linux ;

&#x20;   aucun code Linux n’est présent dans les fichiers Windows ;

&#x20;   le téléchargement est sécurisé ;

&#x20;   le SHA-256 est vérifié avant installation ;

&#x20;   l’installation Linux utilise une élévation de privilèges ponctuelle ;

&#x20;   l’interface Linux n’est pas exécutée en root ;

&#x20;   les fichiers temporaires sont supprimés ;

&#x20;   le packaging Linux est propre ;

&#x20;   le fichier .desktop est installé correctement ;

&#x20;   les tests de sécurité sont concluants.



12\. Instructions finales pour l’IA d’implémentation

L’IA chargée d’implémenter ce plan doit :



&#x20;   respecter strictement la séparation des fichiers par OS ;

&#x20;   utiliser Fyne pour l’interface graphique ;

&#x20;   ne jamais mélanger logique UI et logique système ;

&#x20;   ne jamais exécuter un fichier téléchargé avant vérification ;

&#x20;   ne jamais utiliser de commande shell non maîtrisée ;

&#x20;   ne jamais laisser de fichiers temporaires ;

&#x20;   ne jamais logger de secrets ;

&#x20;   prévoir une gestion d’erreur claire ;

&#x20;   prévoir un nettoyage systématique ;

&#x20;   privilégier un vrai paquet .deb pour Linux ;

&#x20;   refuser toute solution simpliste qui contournerait la sécurité.



13\. Résumé de la décision technique

La solution retenue est :



&#x20;   interface graphique unique avec Fyne ;

&#x20;   séparation de la logique avec build tags Go ;

&#x20;   conservation de la logique Windows dans des fichiers Windows ;

&#x20;   création d’une logique Linux sécurisée dans des fichiers Linux ;

&#x20;   téléchargement du .deb RustDesk ;

&#x20;   vérification SHA-256 avant installation ;

&#x20;   installation via pkexec ;

&#x20;   configuration dans le dossier utilisateur ou système selon le cas ;

&#x20;   nettoyage systématique ;

&#x20;   packaging du configurateur sous forme de .deb avec fichier .desktop.



FIN DU DOCUMENT

