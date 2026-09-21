# Keygen CLI

Outil en ligne de commande pour la génération et la gestion des clés de licence RustDesk.

## Prérequis
- Go 1.21+
- Base de données SQLite configurée

## Compilation
```bash
go build -o keygen .
```

## Utilisation

### Afficher les tarifs et calculer
```bash
./keygen pricing
./keygen pricing --technicians 50
```

### Générer une licence
```bash
# Plan Starter (10€/mois, 1 technicien, viewers illimités)
./keygen generate --email "client@exemple.fr" --plan starter

# Plan Pro (20€/mois, jusqu'à 10 techniciens, viewers illimités)
./keygen generate --email "client@exemple.fr" --plan pro

# Plan Ultra (ex: 20 techniciens = 33,88€/mois, viewers illimités)
./keygen generate --email "client@exemple.fr" --plan ultra --technicians 20
```

### Lister les licences
```bash
./keygen list --status active
./keygen list --email "client@exemple.fr"
```

### Révoquer une licence
```bash
./keygen revoke --id "MP-XXXX-XXXX-XXXX" --reason "Remboursement"
```

### Prolonger une licence
```bash
./keygen extend --id "MP-XXXX-XXXX-XXXX" --days 30
```

### Vérifier une licence
```bash
./keygen check --key "mpsk_..."
```

### Statistiques
```bash
./keygen stats
```

### Export CSV
```bash
./keygen export --file export.csv
```
