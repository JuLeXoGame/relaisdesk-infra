# Integration Tests

Les tests utilisent le vrai schema `database/schema.sql` via le package
`database`, et les vrais handlers API du module `api`.

## Lancer

```bash
go test -v ./...
```

## Couverture

- Cycle de vie licence : creation, activation API, validation API,
  connexion/deconnexion DB, revocation.
- Licence expiree.
- Depassement `max_connections`.
- Performance de validation.
- Resilience DB.
- Parcours de préproduction : achat, paiement, activation technicien, code
  client, activation client et autorisations réseau Ed25519.
- Vérification que le nombre acheté devient `max_sessions`, c'est-à-dire une
  limite de connexions simultanées et non une limite d'installations.

Les flux RustDesk utilisent directement `hbbs`/`hbbr` ; aucun module SOCKS5
n'est requis.

Le script `scripts/preproduction-check.ps1` combine ces tests avec les tests du
fork serveur qui exercent le quota simultané. Son option `-LiveApiUrl` ajoute
les sondes de l'API déployée et des ports hbbs/hbbr.
