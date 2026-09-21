# RelaisDesk - Installeur Windows

Installeur Windows compose de deux parties :

- `configurator/` : application Go native qui active la licence et configure
  RustDesk.
- `nsis/installer.nsi` : script NSIS qui produit `RelaisDesk_Setup_1.0.0.exe`.

## Contrat d'activation

Le configurateur appelle :

```http
POST https://api.relaisdesk.fr/api/v1/technician/login
Content-Type: application/json
```

```json
{"license_id":"MP-...","license_key":"..."}
```

La reponse fournit `license_id`, `server_ip`, `rendezvous_port`, `relay_port`
et la cle publique RustDesk.

## Configuration RustDesk

Le configurateur ecrit `RustDesk2.toml` pour joindre directement le serveur
RustDesk Community :

```toml
rendezvous_server = 'api.relaisdesk.fr:21116'
nat_type = 1
serial = 0

[options]
custom-rendezvous-server = 'api.relaisdesk.fr'
relay-server = 'api.relaisdesk.fr'
api-server = 'https://api.relaisdesk.fr'
key = '{public_key}'
relaisdesk-token-file = '<chemin-prive>/network-token'
relaisdesk-proof-key-file = '<chemin-prive>/proof-key'
```

La configuration ne contient ni proxy SOCKS5 ni `disable-udp`. `api-server`
désigne ici l'API HTTPS RelaisDesk ; il n'active aucun proxy ou composant
RustDesk Server Pro. Le port `21116/udp` doit être autorisé côté serveur.

Les launchers refusent tout binaire dont l'empreinte n'a pas été injectée au
build. Un RustDesk officiel ne comprend pas les preuves d'autorisation et ne
doit jamais être embarqué.

## Build

Pour reconstruire et tester le fork Windows local (recette Sciter non signée),
depuis la racine du projet :

```powershell
.\scripts\build-rustdesk-windows.ps1
```

Ce script épingle la chaîne native, exécute les tests du client, fabrique le
packer portable et reconstruit les deux launchers avec l'empreinte du fork.
Options utiles : `-SkipNativeDependencies`, `-SkipTests` et `-SkipLaunchers`.

Le build de publication multiplateforme reste volontairement bloquant :

```powershell
cd installer
.\build.ps1 `
  -RustDeskForkWindowsPath C:\build\relaisdesk-rustdesk.exe `
  -RustDeskForkLinuxDebPath C:\build\relaisdesk-rustdesk.deb `
  -RustDeskForkLinuxBinaryPath C:\build\rustdesk `
  -ReleaseSigningKeyPath C:\RelaisDesk-Secrets\release-signing-ed25519 `
  -ReleasePublicKey <cle-publique-base64url>
```

`RustDeskForkWindowsPath` doit désigner le portable auto-extractible produit
par la CI, pas le seul lanceur Flutter brut qui dépend de ses DLL et de son
dossier `data`. `RustDeskForkLinuxBinaryPath` désigne le lanceur ELF extrait du
DEB sous `usr/share/rustdesk/rustdesk` ; sa petite taille est normale, le code
principal étant dans `librustdesk.so`.

Options :

```powershell
.\build.ps1 -RustDeskForkWindowsPath <exe> -RustDeskForkLinuxDebPath <deb> -RustDeskForkLinuxBinaryPath <elf> -ReleaseSigningKeyPath <cle-privee> -ReleasePublicKey <cle-publique> -SkipNSIS
.\build.ps1 -Clean
.\build.ps1 -Verbose
```

Le script valide les formats PE, DEB et ELF64 x86_64, calcule les SHA-256,
copie les artefacts dans les launchers, les injecte dans les binaires Go et
crée un manifeste Ed25519 signé. Les sommes et le manifeste utilisent une
liste blanche des six sorties effectivement liées par le site : aucun ancien
fichier présent dans `downloads` n'est repris implicitement. Voir
`docs/RELEASE_SIGNING.md`. Les fichiers sous `embedded/` présents dans le dépôt
ne sont pas des artefacts de publication tant que ce build n'a pas été exécuté
avec le fork compilé.

## Assets

`assets/icon.ico` est un placeholder local. Remplacez-le par l'icone de marque
finale avant release.

## Signature

La voie de production retenue est SignPath Foundation. Le build local reste une
recette non signée. Après acceptation du projet, la CI publique doit faire
signer le fork RustDesk avant son intégration, puis les launchers, reconstruire
NSIS à partir des exécutables internes signés et faire signer les installateurs
finaux. Générer le manifeste Ed25519 seulement après cette dernière signature.

Voir [`../docs/SIGNPATH.md`](../docs/SIGNPATH.md) et
[`../docs/RELEASE_SIGNING.md`](../docs/RELEASE_SIGNING.md). Ne pas acheter de
certificat EV et ne jamais ajouter un `.pfx` au dépôt ou aux secrets GitHub.
