# Correctif Technicien Windows 1.0.0 du 26 octobre 2026

Le numéro produit reste **1.0.0**. Cette reconstruction remplace uniquement les
deux fichiers Technicien Windows du lot du 23 octobre. Les dix autres programmes
conservent leurs empreintes. Le manifeste public signé distingue les lots par
leur date et leurs empreintes SHA-256.

## Sources

- API, lanceur et script Windows : dépôt `JuLeXoGame/relaisdesk-infra`, commit
  `af47dab659e916d69223c95f6fb94a6c0e4056a7`.
- Moteur client inchangé : dépôt `JuLeXoGame/relaisdesk`, commit
  `1054d0f09e4e1d4aaea1309af714289e6055ba4b`, avec ses sous-modules.
- Les autres binaires du lot restent décrits dans
  [la reconstruction du 23 octobre](RECONSTRUCTION_1.0.0_2026-10-23.md).

Le catalogue présenté dans Équipe provient maintenant du parc de la licence
sélectionnée. L'API vérifie que la licence appartient au client connecté avant
de renvoyer ses dossiers. Le dialogue recharge les noms à chaque ouverture et
affiche le chemin des sous-dossiers. Les autorisations déjà enregistrées restent
visibles même lorsqu'un dossier n'est plus disponible.

## Construction Windows

Go 1.26.10, Python 3, MinGW-w64 et NSIS 3 sont nécessaires. Construire le moteur
selon les instructions du 23 octobre et utiliser le reçu produit par
`scripts/build-rustdesk-windows.ps1`. L'option `-TechnicianOnly` conserve la
préparation des deux modules et des contrôles du moteur, puis teste, vérifie et
compile uniquement le Technicien. Les dossiers de travail et de sortie doivent
être nouveaux.

```powershell
$receipt = Get-Content -LiteralPath '<recu-moteur.json>' -Raw | ConvertFrom-Json
& scripts/build-windows-test-release.ps1 `
  -TechnicianOnly `
  -WindowsPortable $receipt.portable `
  -PortableSHA256 $receipt.portable_sha256 `
  -WindowsNativeExecutable $receipt.native `
  -NativeSHA256 $receipt.native_sha256 `
  -WorkingDirectory '<nouveau-dossier-de-travail>' `
  -OutputDirectory '<nouveau-dossier-de-sortie>' `
  -ReleasePublicKey K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo `
  -Version 1.0.0
```

Le moteur portable réutilisé a l'empreinte
`508044659fc082792caf999322b53e5a9451d6f50a2fdaac53a005b91911b02c` ;
le moteur natif du reçu a l'empreinte
`f4d880840ea1f2de8e3039171155a20c26894c92bdb7785d82942d2e497cc943`.

Résultats de la reconstruction publiée :

| Fichier | SHA-256 |
| --- | --- |
| `RelaisDesk_Technicien_Portable.exe` | `557e4cd77ec37385b09f8cde4affcf23237e0d11e83881a4ccb344c1d199af86` |
| `RelaisDesk_Technicien_Setup_1.0.0.exe` | `bf7b4dca166580ade7fae6201ff991ed059de52599fd623d6e009bbd5632854d` |

L'extraction de l'installateur retrouve exactement le même `configurator.exe`
que le fichier portable. Les tests du module et `go vet` passent avant
l'assemblage NSIS. Le programme doit être remplacé manuellement si la version
1.0.0 précédente est déjà installée.

## Construction API Linux

Depuis `api/`, avec Go 1.26.10 :

```sh
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go build -trimpath -mod=readonly -buildvcs=false -ldflags '-s -w' -o api .
```

Le binaire Linux construit a l'empreinte
`46d2201bb2c35932b34d986a4e1faca3814d32e988e0bd5df2c90d32c683a370`.
Ce correctif ne nécessite aucune migration de base. Les appels sans
`license_id` au catalogue client conservent leur comportement existant.

Les clés privées, les accès serveur et les données métier ne sont nécessaires
ni pour compiler ces sources ni pour exécuter leurs tests.
