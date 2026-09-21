# Tester RelaisDesk 1.0.1 avant publication

## État et précautions

Les nouveaux clients sont uniquement locaux. Ne pas les envoyer sur Oracle,
OVH ou une release publique avant la recette et un accord explicite de publication.
Les téléchargements publics restent en `1.0.0-legal.20260907`.
Les corrections serveur Oracle ont déjà été appliquées lors de l'étape précédente ;
cette recette ne demande aucun changement du serveur, de ses clés ou du DNS.

Utiliser deux ordinateurs de test, ou deux machines virtuelles, avec une licence
existante autorisée. Les lanceurs utilisent l'API réelle : les connexions et
l'historique de test peuvent donc y être enregistrés.

Fermer les sessions RustDesk/RelaisDesk existantes avant de commencer. Même la
version « portable » n'est pas isolée : elle peut arrêter les processus RustDesk,
remplacer le moteur sous `%LOCALAPPDATA%\RelaisDesk\bin` et modifier la configuration
de l'utilisateur. Préférer des machines dédiées ou des VM avec instantané ;
conserver les anciens installateurs pour revenir en arrière. Aucun installateur
n'a été exécuté automatiquement pour cette vérification.

## Fichiers à utiliser

Dans le dossier du projet :

`output/security-2026-09-08/release-1.0.1/downloads`

| Usage | Fichier |
| --- | --- |
| PC Windows technicien | `RelaisDesk_Technicien_Portable.exe` |
| PC Windows assisté | `RelaisDesk_Portable.exe` |
| Installation Windows complète, dans un second temps | `RelaisDesk_Setup.exe` |
| Installation Windows technicien, dans un second temps | `RelaisDesk_Technicien_Setup_1.0.0.exe` |
| Linux amd64 technicien | `RelaisDesk_Technicien.deb` |
| Linux amd64 assisté | `RelaisDesk_viewer.deb` |

Le nom du dernier installateur technicien contient encore `1.0.0` pour garder
le lien stable ; sa version interne est bien `1.0.1`.
Ne pas utiliser le paquet Flutter Windows du dossier `client/windows` : le
moteur choisi pour cette livraison reste Sciter, déjà intégré aux lanceurs ci-dessus.

## Essai Windows en priorité

1. Copier le portable technicien sur le premier PC et le portable client sur le
   second. Commencer sans installer les paquets Setup.
2. Ouvrir le technicien, se connecter avec une licence valide, puis ouvrir le
   client et établir une session via la procédure habituelle. Ne pas changer
   manuellement les clés ou les fichiers TOML pour contourner une erreur.
3. Vérifier l'affichage, clavier/souris, presse-papiers dans les deux sens et
   transfert d'un petit fichier sans donnée personnelle. Vérifier le contenu
   du fichier reçu.
4. Garder la session ouverte **au moins dix minutes** et interagir après la
   cinquième minute pour tester le renouvellement d'autorisation.
5. Fermer proprement la session, se déconnecter du technicien, relancer les deux
   applications et vérifier qu'une seconde connexion fonctionne.
6. Si possible, refaire l'essai depuis deux réseaux différents pour ne pas
   valider uniquement une connexion locale.
7. Contrôler que l'intervention apparaît correctement dans l'historique, sans
   laisser de session active après fermeture.

Pour vérifier le quota : avec une licence limitée à N techniciens simultanés,
ouvrir N connexions sur des postes de test, puis une N+1e. L'excédent doit être
refusé ; après déconnexion propre d'un poste, le suivant doit pouvoir se connecter.
L'installation sur plusieurs postes, sans connexion simultanée, reste autorisée.
Ne modifier ni l'offre commerciale ni les règles du serveur pour ce test.

Après réussite des portables, tester les Setup sur une VM avec instantané :
installation, raccourcis, lancement, désinstallation puis réinstallation.
Tester les paquets Linux séparément sur une machine amd64 prévue pour les essais.

## Points connus pendant cette recette

- Le bouton de recherche de mise à jour du technicien peut afficher
  « format de version invalide » : le canal public porte encore le suffixe
  `1.0.0-legal.20260907`, alors que le comparateur existant attend trois nombres.
  Cette incompatibilité est constatée dans le code, pas lors d'un essai graphique.
  Elle ne prouve pas un échec de la signature ni de la connexion distante.
  La correction de cette compatibilité et son test restent à traiter ; ne pas
  publier les nouveaux clients simplement pour masquer cette erreur.
- Le manifeste est signé en Ed25519, mais cela ne constitue pas une signature
  Windows Authenticode. Les `.exe` peuvent afficher « éditeur inconnu » ou une
  alerte SmartScreen. Ne pas désactiver globalement Defender ou SmartScreen.
- Les tests automatisés et les vérifications de paquets sont réussis ; aucune
  recette graphique entre deux machines n'est encore déclarée réussie.

## Retour attendu

Indiquer les versions de Windows/Linux, le fichier lancé, les étapes réussies,
la durée de session et le message exact en cas d'échec. Une capture est utile,
en masquant licence, mots de passe et informations personnelles.

La publication restera suspendue jusqu'à validation et accord explicite.
