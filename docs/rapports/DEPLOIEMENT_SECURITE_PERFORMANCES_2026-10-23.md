# Déploiement sécurité et performances — 23 octobre 2026

## Périmètre

Déploiement autorisé par l'exploitant sur Oracle et publication des programmes
Windows, Linux et macOS.
Version applicative **1.0.0 conservée**. OVH exclu explicitement. Publication des
sources sur les branches GitHub `codex/security-perf-1-0-20261023` autorisée
séparément ; aucun secret n'a été ajouté aux commits.

## API et outils — déployés

- API active : SHA-256 `f456656bd93d72e0df3be8902bd15b9f8c07e74f3bbdd1b9361e4d4523991f73`.
- Go 1.26.10 ; exécutables `api`, `relaisdesk-backup`, `relaisdesk-dbcheck`,
  `relaisdesk-emailcheck` reconstruits et vérifiés après transfert.
- Retour arrière privé : `/opt/relaisdesk/deployments/followup-api-20261023-VJ59NzUY`.
- Migration répétée sur copie SQLite, puis sauvegarde cohérente avant bascule.
  Intégrité et clés étrangères validées. Données métier avant/après : 1 licence,
  2 appareils, 1 commande, 7 comptes.
- Tarifs, essais et quotas publics conservés. API saine, version 1.0.0, base
  connectée. `/api/v1/admin/licences` anonyme : HTTP 401.
- Supervision publique opérationnelle pour API, connexion distante et courriels.

Les sorties locales ont aussi été actualisées : API Windows, outils keygen Linux
et Windows, interface d'administration, debpack et programmes de `installer/build`.
L'interface d'administration et keygen ne sont pas ajoutés aux téléchargements
publics. Aucun générateur n'a été exécuté pour créer de nouvelles clés ou licences.
Copies locales précédentes : `.cache/deploy-20261023/local-programs.previous/`.

## Nginx — déployé

Vérification du certificat TLS de l'API activée pour les deux blocs proxy, dont
les téléchargements. Certificat, nom d'hôte et chaîne vérifiés avant modification.
Configuration Nginx valide ; santé et téléchargements vérifiés après rechargement.
Sauvegarde : `/opt/relaisdesk/deployments/followup-tls-20261023-PetvXnpn`.

## hbbs et hbbr — déployés

- Image : `relaisdesk/rustdesk-server:followup-20261023-amd64`.
- Identité : `sha256:fa62897377936d3e94a1f7f5e82dfee7ce48bca56bca1660c7c374bbbff61eaf`.
- hbbs : `e5bdf0f866cca2ef793a7b5b3513ff96f4c80e5534aeaaa39d90b8a8e4c1255c`.
- hbbr : `1a4ea3162c82724ba912617fe713178a6615f84ad77a7091f5d8b2beebebf332`.
- Sources : [adca9567](https://github.com/JuLeXoGame/rustdesk-server/tree/adca9567eaf9e865ac2085877c953856016d1c68), avec sous-modules.
- 15 tests serveur réussis sous Linux ; compilation de production vérifiée.
- Sauvegarde : `/opt/relaisdesk/deployments/followup-server-20261023-nX4ajs5i`.
- Autorisation obligatoire, clés, ports, racines en lecture seule et VPN conservés.
  Empreintes vérifiées dans les conteneurs ; aucun redémarrage automatique à la
  bascule. L'API et les téléchargements n'ont pas été remplacés par cette étape.
- Métadonnée `SERVER_SOURCE_URL` de l'API alignée sur la révision déployée.
  Sauvegarde privée : `/opt/relaisdesk/deployments/followup-sources-20261023-2iOGaxUz`.

## Programmes — publiés sur Oracle, version 1.0.0

Les douze programmes Windows/Linux/macOS et `SHA256SUMS.txt` sont publiés.
Le manifeste Ed25519, clé existante `release-1`, porte la date
`2026-10-23T08:20:27Z`. Son SHA-256 est
`7454c9aa23424754604f537cfa235899540a6346cdc627c98a848d779af344f6`.
Signature, noms, tailles et empreintes des treize artefacts ont été vérifiés
indépendamment sous Linux, puis deux fois sur Oracle avant la bascule.

Sauvegarde privée des anciens téléchargements et de la configuration :
`/opt/relaisdesk/deployments/followup-downloads-20261023-Pe4pIxO3`.
`CLIENT_SOURCE_URL` pointe désormais sur le commit client `1054d0f09`.
Le [manifeste public](https://api.relaisdesk.fr/api/v1/public/releases/latest)
et le fichier servi par le téléchargement correspondent au manifeste local.
Les [empreintes détaillées](../../relaisdesk/downloads/SHA256SUMS.txt) identifient
chaque programme.

Le contrôle natif complémentaire a identifié un codec AV1 ancien dans la chaîne
de compilation. Le port AOM est passé à **3.14.1**, révision officielle
`03087864cf4bea6abb0d28f95cf7843511413d8f`. Le test `tests/aom_svc_bounds.c`
vérifie le rejet des identifiants SVC négatifs ou hors limites et l'acceptation
des valeurs valides ; il réussit sur la nouvelle bibliothèque Linux.

Référence : [CVE-2026-56209, état Canonical](https://ubuntu.com/security/CVE-2026-56209)
et [correctif amont](https://aomedia.googlesource.com/aom/+/a93ba0ffaacd5f576a241bf739110e65287e516d).

FFmpeg passe de **7.1 à 7.1.5**, sans changement de branche majeure de son API.
Les correctifs spécifiques du fork s'appliquent tous à cette version.
Archive officielle GitHub n7.1.5, SHA-512 :
`eddcd57e4f048f62a95ccbf4ed9ef713d56fe22d4ecaac4175ed3ad8d664c2552f547d668662be04dc469f96109872749bcbbdd2412d3c15d2fa797565028e16`.
Voir les [versions maintenues](https://ffmpeg.org/download.html) et les
[avis de sécurité FFmpeg](https://ffmpeg.org/security.html).

Sources du moteur client distribué :
[1054d0f09](https://github.com/JuLeXoGame/relaisdesk/tree/1054d0f09e4e1d4aaea1309af714289e6055ba4b).
Compilation macOS ARM/Intel :
[exécution GitHub Actions](https://github.com/JuLeXoGame/relaisdesk-infra/actions/runs/39916359240).
Les moteurs natifs de cette exécution ont été compilés et vérifiés. Leur premier
assemblage invalidait la signature en rattachant le moteur au nouvel Info.plist
du lanceur. Le moteur est désormais
conservé dans sa propre application signée sous `Contents/Helpers`, selon les
[emplacements de code imbriqué documentés par Apple](https://developer.apple.com/library/archive/technotes/tn2206/_index.html).
L'assemblage final reprend ces moteurs dans
[l'exécution 39920735454](https://github.com/JuLeXoGame/relaisdesk-infra/actions/runs/39920735454).
Les deux architectures ont réussi leurs tests Go, `go vet`, contrôles de
signature, architecture, montage des DMG et intégrité du moteur embarqué.
Les quatre DMG contiennent les applications complètes et le service natif.
Les lanceurs macOS sont compilés depuis `1cb44f9` ; les sources et scripts finaux,
incluant les dépendances Linux, sont accessibles au commit
`3fff1a65eea8f340f700c45ee829b8f1833872db` du dépôt `relaisdesk-infra`.
Les signatures Mac sont ad hoc : aucune notarisation Apple ni signature
Developer ID n'est revendiquée. Minimums : macOS 12.3 Apple et macOS 12.0 Intel.

Windows et Linux sont construits. Les tests des deux lanceurs ont été exécutés
avec les nouveaux moteurs et leurs empreintes injectées. Les installateurs NSIS
ont été extraits sans installation ; leur contenu correspond aux exécutables
portables vérifiés. Les deux paquets Debian contiennent exactement les exécutables
annoncés en version 1.0.0. Le Technicien Linux utilise CGO ; ses dépendances GL,
X11 et Wayland sont déclarées. Le paquet du moteur natif déclare aussi glibc 2.35
minimum, libstdc++ 11 minimum et libgcc-s1 ; ses exécutables sont inchangés après
cette correction de métadonnées. SHA-256 du DEB moteur final :
`c3e0d52b4f4657d83fc421ef160c13c61e1d1658607eba1c3a5cddd0ad922853`.
Les deux lanceurs ont été reconstruits et retestés avec cette empreinte.
`ldd -r` ne signale ni bibliothèque manquante ni symbole non résolu.
Un démarrage du Technicien Linux sous affichage virtuel Xvfb avec rendu logiciel
réussit : fenêtre 1020 × 640 détectée, processus actif et aucun échec graphique
fatal. Ce contrôle utilise des répertoires de données et configuration de test.
Le moteur Flutter Linux embarqué démarre également : fenêtre 800 × 600 et pont
natif chargé sans erreur fatale. Ce second test utilise un espace de processus et
un réseau isolés, sans accès aux services de production.

Les [instructions de reconstruction](../RECONSTRUCTION_1.0.0_2026-10-23.md)
documentent les entrées de compilation et les contrôles effectués.
Après publication, les contrôles externes confirment : API saine en 1.0.0 et base
connectée ; téléchargements Windows et Linux HTTP 206 avec les tailles finales ;
administration anonyme HTTP 401 ; accès Mac anonyme HTTP 403, protection bêta
conservée. L'API et les conteneurs hbbs/hbbr sont actifs, sans redémarrage
automatique. L'empreinte de l'API déployée reste celle indiquée plus haut.

Le numéro de version restant 1.0.0, les clients installés dont le contrôle de mise
à jour compare seulement le numéro ne proposeront pas automatiquement ce nouveau
lot. Ils doivent être remplacés par les nouveaux programmes pour recevoir les
correctifs. Les tests automatisés et les vérifications des paquets ne constituent
pas un essai complet d'une session distante graphique sur chaque système.

## Site OVH — fichiers locaux uniquement

Fichiers préparés dans `relaisdesk/`, sans transfert OVH :

- `admin/sw.js` et `technicien/sw.js` ;
- `client/index.html` et `client/.htaccess` ;
- `logiciel-libre.html` : sources des moteurs, lanceurs et instructions de reconstruction ;
- `downloads/` : douze programmes, empreintes et manifeste identiques au lot Oracle.

Aucune archive ZIP du site ni copie du site dans `output/` n'a été créée.
La copie temporaire de clé SSH utilisée pour le déploiement a été supprimée après
les contrôles ; les clés originales sont conservées.

## Retour arrière

Les répertoires privés conservent les exécutables, configurations et sauvegardes
nécessaires. Ne pas restaurer automatiquement les bases de production : cela
effacerait les écritures postérieures à la bascule. Les scripts prévoient le
retour aux exécutables/configurations précédents si les contrôles échouent.
