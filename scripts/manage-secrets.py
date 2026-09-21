#!/usr/bin/env python3
"""Local key inventory and rotation guidance. Never changes a key or contacts Oracle."""
from __future__ import annotations

import argparse
from pathlib import Path
import sys

PROJECT_ROOT = Path(__file__).resolve().parent.parent
SUPPORTED_TARGETS = {
    "admin_token": "Invalider les acces utilisant l'ancien jeton, sauvegarder api.env avec droits restreints, changer une seule valeur, verifier l'API et prevoir un retour arriere.",
    "trial_fingerprint": "NE PAS regenerer : la base lie les marqueurs anti-abus a cette cle HMAC. Restaurer la cle existante ; une migration metier specifique est necessaire pour en changer.",
    "release_signing": "Distribuer d'abord une version cliente acceptant la nouvelle cle tout en conservant l'ancienne. Verifier le manifeste courant et la cle attendue par l'API avant la bascule. Une simple regeneration casserait les mises a jour.",
    "network_auth": "Ajouter la nouvelle cle avec un nouvel identifiant aux cles acceptees par hbbs ET hbbr, recreer/verifier les conteneurs, puis basculer le signataire API. Conserver l'ancienne cle pendant au moins la validite maximale des jetons et verifier les connexions avant retrait.",
    "backup_key": "Conserver les anciennes cles hors du VPS et leur correspondance avec les archives. Tester une restauration avec chaque generation avant toute rotation ; ne jamais ecraser la seule cle de dechiffrement.",
}


def regenerate_secret(*args, **kwargs):
    # Fail closed even for callers that previously imported this function.
    raise ValueError("Rotation automatique desactivee. Utiliser --action plan ; aucune cle n'a ete changee.")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--action", choices=("show", "plan", "regenerate"), default="show")
    parser.add_argument("--target", choices=tuple(SUPPORTED_TARGETS) + ("all",), default="all")
    args = parser.parse_args(argv)
    if args.action == "regenerate":
        regenerate_secret()
    targets = SUPPORTED_TARGETS if args.target == "all" else {args.target: SUPPORTED_TARGETS[args.target]}
    if args.action == "show":
        print("Inventaire LOCAL uniquement (aucune lecture du contenu des secrets, aucune connexion Oracle).")
        for name in ("release-signing-ed25519", "release-signing-public.txt", "network-auth-ed25519", "backup.key"):
            path = PROJECT_ROOT / ".secrets" / name
            print(name + " : " + ("present" if path.is_file() else "absent de .secrets"))
        print("Cet inventaire ne valide ni les cles ni leur correspondance avec la production.")
    for target, instructions in targets.items():
        print(target + " : " + instructions)
    print("Pas de rotation globale. La maintenance de dependances ne necessite aucune regeneration de cle.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ValueError as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
