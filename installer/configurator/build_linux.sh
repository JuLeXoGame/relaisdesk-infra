#!/bin/bash
set -euo pipefail

# Ce script compile et génère le paquet Debian natif pour Linux.
# Pré-requis : go, gcc, fyne.io/fyne/v2/cmd/fyne

# -----------------------------------------------------------------------
# APIURL par défaut : domaine professionnel en HTTPS vérifié
# Ancien domaine DuckDNS : https://relaisdesk.duckdns.org:8443
# -----------------------------------------------------------------------
APIURL="${APIURL:-https://api.relaisdesk.fr}"
RUSTDESK_FORK_LINUX_DEB="${RUSTDESK_FORK_LINUX_DEB:?chemin du paquet .deb du fork requis}"
RUSTDESK_FORK_LINUX_BINARY="${RUSTDESK_FORK_LINUX_BINARY:?chemin du binaire ELF du fork requis}"

test -f "$RUSTDESK_FORK_LINUX_DEB"
test -f "$RUSTDESK_FORK_LINUX_BINARY"
cp -- "$RUSTDESK_FORK_LINUX_DEB" embedded/rustdesk.deb
RUSTDESK_PACKAGE_SHA256="$(sha256sum "$RUSTDESK_FORK_LINUX_DEB" | awk '{print $1}')"
RUSTDESK_BINARY_SHA256="$(sha256sum "$RUSTDESK_FORK_LINUX_BINARY" | awk '{print $1}')"

echo "🔍 Vérification des pré-requis..."
if ! command -v go &> /dev/null; then
    echo "❌ Erreur: 'go' n'est pas installé."
    exit 1
fi
if ! command -v fyne &> /dev/null; then
    echo "📥 Installation de l'outil fyne..."
    go install fyne.io/fyne/v2/cmd/fyne@latest
    export PATH="$PATH:$(go env GOPATH)/bin"
fi

echo "📦 Nettoyage et téléchargement des dépendances..."
go mod tidy

echo "🏗️ Compilation avec APIURL=$APIURL"
export GOFLAGS="-ldflags=-X main.APIURL=$APIURL -X main.APP_VERSION=1.0.0 -X main.RELEASE_PUBLIC_KEY=K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo -X main.RUSTDESK_EXPECTED_SHA256=$RUSTDESK_BINARY_SHA256 -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=$RUSTDESK_PACKAGE_SHA256"

echo "🏗️ Empaquetage du configurateur (Fyne) en paquet .deb..."
# L'outil fyne package se charge de compiler pour Linux, d'intégrer l'icône, 
# et de créer le fichier .desktop pour le menu des applications.
fyne package -os linux -pkg deb -name relaisdesk-configurator -appID com.relaisdesk.configurator -icon ../assets/icon.png

echo "✅ Succès ! Le paquet .deb a été généré avec succès dans le dossier actuel."
echo "Pour l'installer : sudo dpkg -i relaisdesk-configurator.deb"
