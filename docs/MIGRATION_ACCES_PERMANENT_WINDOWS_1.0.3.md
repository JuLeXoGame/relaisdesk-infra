# Migration locale de l'acces permanent Windows vers 1.0.3

Preparation du 12 septembre 2026, pour le poste confirme par l'exploitant :
service `RustDesk` dans `C:\Program Files\RustDesk`, et service `RelaisDeskFleet`
dans `C:\ProgramData\RelaisDeskFleet`. L'exploitant est physiquement devant le PC
et ne souhaite pas sauvegarder l'ancien moteur.

## Ce que fait le kit

- Verifie les quatre composants par SHA-256, les chemins, les comptes systeme,
  les fichiers d'identite et l'absence de liens/jonctions dans les chemins.
- Arrete temporairement les deux services et les processus de leurs chemins
  moteur confirmes ; une session distante sera coupee.
- Installe le moteur verifie et ses deux DLL dans `Program Files\RelaisDeskEngine`
  avec des droits reserves a SYSTEM/Administrateurs (lecture pour LocalService).
- Remplace l'agent par le Viewer 1.0.3 et modifie le chemin du service existant.
- Conserve `state.json`, `proof-key`, la configuration d'identite/mot de passe
  `RustDesk.toml`, ainsi que la fiche du poste dans la console. Aucun `--unenroll`,
  aucun nouveau code permanent, aucune suppression/recreation de fiche.
- Demarre l'agent qui demande l'autorisation a l'API existante avant de demarrer
  le moteur. Attend au maximum 60 secondes le marqueur d'autorisation et les
  deux services demarres. Ce controle ne prouve pas une connexion distante reelle.

Aucune copie de sauvegarde de l'ancien moteur/agent n'est creee. L'ancien dossier
`Program Files\RustDesk` n'est ni copie ni supprime : ses fichiers restent en place,
mais le service utilise le nouveau dossier. Ne pas relancer son ancien executable.
L'agent de `ProgramData` est remplace ; il n'y a pas de retour arriere automatique.
En cas d'echec apres debut de la coupure, le script tente de laisser les services
arretes et desactives, sans supprimer l'identite. Transmettre l'erreur pour reprise.

## Utilisation

Le dossier transportable sans ZIP est `installer/build/migration-permanent-1.0.3/`.
Si le poste est un autre PC, copier **tout ce dossier**, avec son sous-dossier
`payload`, par un moyen de confiance. Ne pas telecharger le lot public 1.0.2 a sa
place. Le kit ne modifie pas `relaisdesk/downloads/` et n'est pas publie.

Fermer les fenetres RelaisDesk/RustDesk du poste et les interventions en cours.
Ouvrir PowerShell **64 bits en administrateur**, se placer dans le dossier du kit.

Verification seule, sans changement :

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\migrate-windows-permanent-1.0.3.ps1
```

Migration avec coupure temporaire :

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\migrate-windows-permanent-1.0.3.ps1 -Apply
```

`ExecutionPolicy Bypass` ne s'applique qu'a ce processus PowerShell : aucune
politique persistante ni protection Defender n'est desactivee. Le script n'est
pas signe ; ne lancer que le kit prepare et verifie, pas une copie d'origine inconnue.

Apres succes, verifier la fiche existante dans la console, tester une connexion
depuis le Technicien 1.0.3, puis redemarrer Windows et refaire le test. Aucune
intervention Oracle/API ni publication web n'est requise par cette migration.

## Validation et limites

`scripts/test-windows-permanent-migration.ps1` teste les fonctions sur des fichiers
inertes et des services simules. Il ne lance pas la migration et ne touche pas aux
services reels. Les empreintes du paquet sont celles du
[lot de test Windows 1.0.3](RECETTE_SECURITE_1.0.3_WINDOWS.md).
L'installation sur un poste Windows reel et la reconnexion restent a effectuer
par l'exploitant ; aucun succes reel n'est presume.

References techniques : [Win32_Service](https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-service)
et [remplacement atomique File.Replace](https://learn.microsoft.com/en-us/dotnet/api/system.io.file.replace?view=netframework-4.8.1).
