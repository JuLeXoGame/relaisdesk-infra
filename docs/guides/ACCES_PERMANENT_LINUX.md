# Accès permanent Linux — préparation et recette locale

## État au 10 septembre 2026

L'intégration est écrite dans les sources locales : service systemd, enrôlement signé, renouvellement, retrait et canal local de signature pour le client RustDesk. **Elle n'est pas déployée ni validée sur un véritable poste Linux.** Aucun accès à Oracle, aucun binaire de téléchargement remplacé, aucun envoi GitHub.

Le Viewer a été compilé pour Linux et ses tests spécifiques compilent. Les tests de logique partagée s'exécutent sur Windows. Le PC de développement ne dispose ni de WSL installé, ni d'une VM Linux disponible : les tests du service, du socket Unix et du bureau distant restent à exécuter sur Linux. La petite modification du fork Rust doit aussi être compilée pour Linux avant tout essai réel.

Ce guide concerne **le poste Linux administré à distance**, pas le serveur Oracle. Une machine virtuelle de test et une API/hbbs/hbbr de laboratoire suffisent. Ne pas utiliser les clés, la base ou les paiements de production pour cette recette.

## Prérequis du poste

- Linux avec systemd comme gestionnaire de services ; cible initiale Debian/Ubuntu amd64 avec bureau graphique. Les autres distributions et architectures ne sont pas implicitement certifiées.
- Fork RustDesk installé sous `/usr/bin/rustdesk` (le lien standard vers `/usr/share/rustdesk/rustdesk` est accepté), avec `rustdesk.service` système exécuté par root.
- Compte root dont le dossier personnel est `/root`, configuration du service sous `/root/.config/rustdesk`. Les installations personnalisées avec EnvironmentFile ou RootDirectory sont refusées.
- Identifiant RustDesk réel et mot de passe permanent fort déjà configurés dans le service RustDesk. Le Viewer ne crée pas de mot de passe faible, ne le transmet pas à l'API et ne modifie pas `RustDesk.toml`.
- Heure système correcte et accès HTTPS à l'API de laboratoire, puis accès au réseau RustDesk de laboratoire.

La disponibilité du service n'est pas une preuve de capture d'écran fonctionnelle. Tester le bureau, l'écran verrouillé et l'écran de connexion sur la distribution et la session graphique utilisées. L'intégration ne désactive pas les protections Wayland et ne garantit pas l'accès à tout écran de connexion ni à une machine sans affichage.

## Compiler les bonnes versions

1. Compiler le **fork client modifié** : `rustdesk/src/relaisdesk_auth.rs` appelle désormais `rustdesk/src/relaisdesk_fleet_linux.rs` sous Linux. Aucun changement de hbbs/hbbr n'est requis par cet ajout. Un ancien client est refusé avant consommation du code d'installation.
2. Utiliser le paquet Debian, son exécutable ELF et, pour la version Flutter, **sa propre `librustdesk.so`**. Le Viewer vérifie les empreintes de l'exécutable et de la bibliothèque installés ; le seul petit lanceur Flutter ne suffit pas à identifier le code Rust.
3. Dans `installer/viewer`, le script `build_linux.sh` accepte `RUSTDESK_FORK_LINUX_DEB`, `RUSTDESK_FORK_LINUX_BINARY` et `RUSTDESK_FORK_LINUX_SO`, avec des chemins absolus. Fournir `APIURL` pour le laboratoire. Il produit le Viewer et son paquet localement, sans transfert ni modification d'Oracle. Il embarque le paquet fourni dans `embedded/rustdesk.deb`.
4. Le build PowerShell historique accepte maintenant `-RustDeskForkLinuxSoPath`. Le script de release accepte `-LinuxLibrary`. **Ne pas lancer ces scripts de release pour un simple test** : leurs opérations historiques peuvent préparer ou remplacer des téléchargements locaux. Aucune de ces procédures de release n'a été exécutée ici.
5. Une copie protégée du Viewer est installée en tant qu'agent. Mettre à jour le paquet Viewer ne remplace pas silencieusement cette copie active. Pour cette première version, une mise à niveau de l'agent se fait par retrait explicite puis nouvel enrôlement, avec la recette correspondante.

Avant distribution, publier les sources correspondant exactement au nouveau fork et actualiser les références de sources des nouveaux artefacts. Ne pas réutiliser une ancienne URL de commit pour ce nouveau binaire. La page logiciel libre et les anciens manifestes ne sont pas modifiés tant qu'aucun nouveau binaire n'est publié.

## Installer sur un poste de laboratoire

Installer les paquets de test du fork puis du Viewer et configurer le mot de passe permanent du **service** dans RustDesk. Confirmer localement l'identité, sans afficher les fichiers contenant le mot de passe :

```bash
sudo systemctl status rustdesk --no-pager
sudo /usr/bin/rustdesk --get-id
```

Créer une fiche dans la console de laboratoire. Son code est valable 15 minutes, pour un seul poste. Puis exécuter :

```bash
sudo /usr/bin/relaisdesk-viewer --enroll
```

Saisir le code à l'invite : cela évite de le conserver dans l'historique du shell et les arguments de la commande. Il est également possible de saisir le code `PERM-...` dans un Viewer lancé avec les privilèges requis. Sans ces privilèges, un message explique la commande à utiliser ; aucune élévation implicite n'est exécutée.

L'installation vérifie les prérequis, signe l'enrôlement avec la clé propre au poste, copie l'agent dans son dossier protégé, écrit les unités systemd et attend une première autorisation ainsi qu'une demande de preuve venant du processus RustDesk autorisé. L'attente peut durer jusqu'à 90 secondes. Une erreur n'est jamais présentée comme une installation réussie.

```bash
sudo systemctl status relaisdesk-fleet rustdesk --no-pager
sudo journalctl -u relaisdesk-fleet -u rustdesk --no-pager -n 80
sudo stat -c '%U:%G %a %n' /var/lib/relaisdesk-fleet /var/lib/relaisdesk-fleet/proof-key
```

Le dossier privé doit appartenir à `root:root`, mode `700`, et la clé être en mode `600`. Ne jamais copier le contenu de la clé ou du jeton dans un ticket/journal.

## Fonctionnement et protections

- `relaisdesk-fleet.service` : service système au démarrage, indépendant de la fenêtre Viewer et de la session utilisateur, redémarré sur erreur.
- `/var/lib/relaisdesk-fleet/` : identité du poste, clé Ed25519, jeton de courte durée, copie de l'agent et verrous locaux. Accès root uniquement.
- `/run/relaisdesk-fleet/proof.sock` : socket Unix local, pas un port TCP. Son accès permet aux processus de bureau d'adresser une demande, **pas de récupérer la clé privée**. Le service contrôle les identifiants de processus fournis par le noyau (`SO_PEERCRED`), l'exécutable et sa filiation avec le service RustDesk root. Un client indépendant ou un autre programme est refusé. Les connexions sont bornées en taille, en nombre et en durée.
- La configuration copiée par RustDesk aux utilisateurs contient le chemin du socket, aucun chemin vers la clé privée. Les paramètres existants sont conservés, sauf les options serveur/autorisation et le choix du mot de passe permanent. Le mode d'approbation devient `password` (pas de clic local requis), et `allow-hide-cm=N` conserve l'affichage du gestionnaire de connexion. L'identité et le mot de passe RustDesk restent dans leur fichier d'origine.
- Le complément systemd de RustDesk impose une autorisation non expirée avant démarrage et lie son fonctionnement à celui de l'agent. Le socket refuse de signer un jeton expiré ; une révocation API entraîne l'arrêt du service RustDesk. Les contrôles du fork continuent de s'appliquer aux sessions ouvertes.
- L'ancienne commande d'arrêt global `pkill` du service est remplacée, pour ce mode, par l'arrêt du groupe de processus géré par systemd.
- Le parcours temporaire refuse de reconfigurer ce même poste tant que l'agent permanent est installé. `--stop` ne désinstalle pas le service permanent.

Comme tout logiciel de contrôle distant, ce dispositif ne prétend pas résister à un administrateur root malveillant ni à une compromission du processus RustDesk autorisé. Le statut « agent et service actifs » ne garantit pas qu'une prise en main graphique réussira.

Références techniques : [dépendances systemd, dont BindsTo et After](https://www.man7.org/linux/man-pages/man5/systemd.unit.5.html), [identité des correspondants sur socket Unix](https://www.man7.org/linux/man-pages/man7/unix.7.html).

## Recette indispensable avant publication

1. Exécuter sur Linux les tests Viewer et `go vet`, puis compiler et tester le fork client Linux. Le fichier `fleet_linux_test.go` ne s'exécute pas lors des tests natifs Windows.
2. Installation propre : aucune clé privée lisible par un utilisateur standard, vrai ID dans la console, preuve locale acceptée, connexion depuis un technicien autorisé avec le mot de passe du poste.
3. Fermer le Viewer et le terminal ; vérifier l'accès et le renouvellement pendant plus de cinq minutes.
4. Redémarrer la VM ; tester avant et après ouverture de session, puis verrouillage/déconnexion. Vérifier que le bureau réellement affiché correspond au poste attendu, pour chaque environnement graphique annoncé comme pris en charge.
5. Supprimer la fiche, révoquer/expirer la licence, puis couper l'API au-delà de l'échéance du jeton. Vérifier l'arrêt de l'accès, y compris de la session déjà ouverte. Une panne transitoire avant expiration ne doit pas générer une nouvelle identité.
6. Arrêter puis tuer l'agent : RustDesk doit être arrêté par la dépendance systemd, puis n'être relancé qu'après une nouvelle autorisation. Vérifier les journaux si un processus enfant est déplacé hors du groupe par les règles PAM locales.
7. Essayer de parler au socket depuis un programme ordinaire et depuis un RustDesk lancé indépendamment : refus. Vérifier le cas réel du processus RustDesk enfant lancé via sudo par le service, y compris à l'écran de connexion.
8. Tester code expiré, réutilisation sur une autre identité, installation interrompue, mauvais binaire/bibliothèque, liens symboliques et permissions trop larges. Ne pas corriger un échec en désactivant les vérifications d'empreinte ou en élargissant les droits de la clé.
9. Tester désinstallation, réinstallation et limite de connexions simultanées de l'offre. Le nombre total de postes installés n'est pas limité par cet ajout.

## Retirer l'accès permanent

```bash
sudo /usr/bin/relaisdesk-viewer --unenroll
```

Cela arrête les services concernés, désactive/retire l'unité de parc et son complément RustDesk, puis retire uniquement les fichiers propres à cet enrôlement. Les fichiers système modifiés manuellement ne sont pas effacés automatiquement. Le dossier privé et ses verrous peuvent rester ; aucune suppression récursive n'est réalisée. L'identité et le mot de passe RustDesk ne sont pas supprimés. La configuration conserve le socket d'autorisation désormais absent : l'ancien accès ne redevient pas silencieusement autonome.

Supprimer aussi la fiche dans la console. La suppression seule de la fiche coupe les renouvellements mais ne désinstalle pas physiquement le service du poste.

Le nouveau paquet Debian appelle ce retrait lors d'une **suppression** du paquet Viewer ; une simple mise à niveau du paquet ne supprime pas l'enrôlement. Une installation interrompue se retire de la même manière avant de générer un nouveau code.

## Site web

Seuls `relaisdesk/client/index.html` et `relaisdesk/i18n.js` sont ajustés pour les prérequis Linux. Ils restent dans `relaisdesk/`, sans archive. Attendre les tests et la mise à disposition coordonnée des versions compatibles avant de les transférer sur OVH. Le lien de connexion du portail continue d'utiliser le configurateur installé sur Windows ; cet ajout concerne le poste Linux distant, pas un nouveau gestionnaire d'URI pour technicien Linux.
