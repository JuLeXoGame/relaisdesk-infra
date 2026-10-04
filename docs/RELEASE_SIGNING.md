# Signature des versions RelaisDesk

La signature des versions utilise une paire Ed25519 dédiée. Elle est distincte
de la clé d'identité RustDesk `id_ed25519` et de la clé d'autorisation réseau.
La clé privée de version reste hors du serveur de production et hors de Git.

Cette signature protège le manifeste de téléchargement ; elle ne remplace pas
la signature Windows Authenticode visible par l'utilisateur et vérifiée par
Windows.

## Création initiale

Sur le poste de construction sécurisé :

```powershell
cd api
go run ./cmd/release-keygen -out C:\RelaisDesk-Secrets\release-signing-ed25519
```

La commande refuse d'écraser une clé existante et affiche uniquement
`RELEASE_PUBLIC_KEY`. Sauvegarder la clé privée hors ligne et copier la clé
publique dans la configuration de l'API.

## Construction et signature

```powershell
cd installer
.\build.ps1 `
  -RustDeskForkWindowsPath C:\build\relaisdesk-rustdesk.exe `
  -RustDeskForkLinuxDebPath C:\build\relaisdesk-rustdesk.deb `
  -RustDeskForkLinuxBinaryPath C:\build\rustdesk `
  -ReleaseSigningKeyPath C:\RelaisDesk-Secrets\release-signing-ed25519 `
  -ReleasePublicKey <cle-publique-base64url>
```

Le paramètre Windows reçoit le portable auto-extractible de la CI et non le
petit lanceur Flutter isolé. Le paramètre ELF reçoit le fichier
`usr/share/rustdesk/rustdesk` extrait du DEB ; il peut ne faire que quelques
dizaines de kilo-octets, car le code principal réside dans
`usr/share/rustdesk/lib/librustdesk.so`.

Le script :

1. injecte la clé publique et le numéro de version dans le configurateur ;
2. calcule les SHA-256 des six sorties de distribution explicitement
   autorisées, sans reprendre les fichiers cachés ou périmés du dossier ;
3. génère `release-manifest.json` ;
4. signe son contenu canonique avec Ed25519 ;
5. vérifie immédiatement la signature avant de terminer.

Configurer ensuite l'API :

```text
RELEASE_MANIFEST_PATH=/opt/relaisdesk/downloads/release-manifest.json
RELEASE_PUBLIC_KEY=<meme-cle-publique-base64url>
```

En production, l'API refuse de démarrer sans ces deux valeurs. Lors de chaque
téléchargement, elle vérifie à nouveau la signature, le nom, la taille et le
SHA-256 du fichier. Le configurateur vérifie indépendamment la signature avec
la clé publique intégrée avant d'annoncer une mise à jour.

## Signature Windows Authenticode

Décision du 7 septembre 2026 : la distribution n'est pas bloquée par SignPath.
L'ordre décrit ci-dessous s'applique lorsqu'Authenticode est effectivement
utilisé. Sans Authenticode, calculer et signer le manifeste sur les fichiers
non signés définitifs et les annoncer comme tels ; ne jamais supprimer la
vérification Ed25519 pour contourner une empreinte périmée.

La voie retenue est le programme gratuit **SignPath Foundation pour projets open
source**. Aucune clé Authenticode, aucun certificat EV et aucun fichier `.pfx`
ne doivent être achetés ou stockés par RelaisDesk. La clé privée de signature
reste dans le HSM de SignPath. Le certificat est cependant émis au nom de
`SignPath Foundation`, qui sera l'éditeur affiché par Windows.

Cette décision n'est opérationnelle qu'après acceptation du dossier. Les
conditions, blocages actuels, démarches et paramètres CI sont détaillés dans
[`SIGNPATH.md`](SIGNPATH.md). En particulier, le périmètre distribué doit être
entièrement open source, licencié sous une licence reconnue par l'OSI, construit
depuis des dépôts publics par des runners GitHub hébergés et approuvé
manuellement à chaque release.

L'ordre de production est impératif :

1. faire signer par SignPath le fork RustDesk Windows ;
2. construire les launchers en embarquant ce fork déjà signé, pour que
   l'empreinte compilée corresponde à l'octet près ;
3. faire signer les launchers ;
4. construire NSIS à partir des exécutables internes signés ;
5. faire signer les installateurs NSIS finaux ;
6. calculer seulement ensuite `SHA256SUMS.txt` et le manifeste Ed25519.

Le script `installer/build.ps1` actuel construit NSIS et le manifeste dans une
seule exécution locale. Il reste valable pour la recette, mais la chaîne de
publication devra être découpée dans la CI selon les six étapes ci-dessus avant
la première release SignPath.

Après signature :

```powershell
Get-AuthenticodeSignature .\relaisdesk\downloads\*.exe |
  Format-Table Path, Status, SignerCertificate
```

Chaque statut doit être `Valid` et le sujet du certificat doit correspondre à
SignPath Foundation. Vérifier aussi les `.dll` et les exécutables extraits par
l'installateur dans une VM propre. Une nouvelle version correctement signée
peut tout de même afficher temporairement SmartScreen : la signature SignPath
ne garantit pas une réputation instantanée.

En cas de refus définitif de SignPath, utiliser Azure Artifact Signing comme
premier repli, puis un certificat OV. Ne choisir EV que si un client l'exige
contractuellement, jamais pour promettre un contournement SmartScreen.

Références : [conditions SignPath](https://signpath.org/terms),
[intégration GitHub SignPath](https://docs.signpath.io/trusted-build-systems/github),
[options de signature Windows](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options)
et [réputation SmartScreen](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/smartscreen-reputation).

## Rotation

Les clients vérifient le `key_id` du manifeste contre un trousseau embarqué
(`SelfUpdateCheckWithKeys`, `ReleaseSigningKeyID`/`RELEASE_KEY_ID`, paramètre
`keyID` de `verifyReleaseManifest` côté parc) : un manifeste signé par une clé
hors trousseau est rejeté même si sa signature est valide. Procédure :

1. générer la nouvelle paire (`release-keygen`) sous l'identifiant suivant
   (`release-2`, `release-3`, …) et la sauvegarder hors ligne ;
2. diffuser une version transitoire dont le trousseau contient l'ancienne ET
   la nouvelle clé, signée avec l'ancienne clé pour que les clients actuels
   l'acceptent ;
3. une fois la version transitoire largement diffusée, signer les manifestes
   avec la nouvelle clé (`-KeyId release-2` dans `sign_manifest.ps1`) ;
4. dans une version ultérieure, retirer l'ancienne clé du trousseau : les
   manifestes qu'elle signe cessent alors d'être acceptés.

Ne jamais signer avec la nouvelle clé avant l'étape 3 : les clients non encore
à jour rejetteraient les manifestes. Une rotation périodique n'est pas
nécessaire ; elle est surtout requise en cas de compromission.
