# Changelog & Guide de Retour Arrière : Amélioration du Mode Stretch & Résolution Distante

Ce document détaille les modifications apportées pour améliorer le mode **Stretch** (étirement plein écran) de la vue technicien Windows, ainsi que la procédure exacte pour revenir en arrière en cas de problème.

---

## 1. Objectifs & Problématiques résolues

1. **Étirement complet sans bandes noires** :
   - *Ancien comportement* : Le mode stretch imposait `scale = min(scale_x, scale_y)` et `foreground-size: contain`, ce qui laissait des bandes noires si les ratios d'écran différaient, et ignorait complètement l'étirement si la résolution distante était supérieure ou égale à la fenêtre (`scale <= 1`).
   - *Nouveau comportement* : En mode `stretch`, l'élément vidéo prend 100% de la largeur et 100% de la hauteur du conteneur (`w = bw`, `h = bh`) et applique `foreground-size: 100% 100%` (`video#handler.stretch`). L'image occupe l'intégralité de l'écran ou de la fenêtre.

2. **Élimination de la déformation visuelle via ajustement automatique de la résolution distante** :
   - *Ancien comportement* : Si l'hôte distant avait un ratio différent (ex: 4:3 en 1024x768) de l'écran du technicien (ex: 16:9 en 1920x1080), un simple étirement déformait l'image (cercles ovales, texte écrasé).
   - *Nouveau comportement* : Le technicien envoie une demande d'ajustement de résolution (`change_resolution`) au serveur distant pour faire correspondre le ratio et la résolution à l'écran local du technicien (ex: 1920x1080).
   - Un bouton d'action directe a également été ajouté dans le menu Affichage : **"Adapter à la résolution locale"** (`#fit-local-resolution`).

3. **Précision pixel-perfect de la souris** :
   - *Ancien comportement* : Le calcul de position de la souris utilisait un facteur d'échelle unique `cursor_scale`.
   - *Nouveau comportement* : Découplage complet en `cursor_scale_x` et `cursor_scale_y`. Quel que soit le facteur d'étirement horizontal ou vertical, le clic et la position du curseur distant correspondent exactement aux pixels cliqués par le technicien.

4. **Correction de l'écrasement vertical (mode plein écran)** :
   - *Cause identifiée* : 
     1. `adjustRemoteResolution` utilisait `#workarea` au lieu de `#frame`, ce qui soustrayait la hauteur de la barre des tâches Windows (ex: demandait 1040p au lieu de 1080p natif, écrasant la résolution distante de 40px).
     2. Lors du passage en plein écran, le bandeau d'en-tête était masqué *après* le calcul des dimensions d'affichage, réduisant la hauteur disponible de 38px.
   - *Correction apportée* :
     1. `adjustRemoteResolution` cible désormais toujours `#frame` (la vraie résolution physique 16:9 du moniteur, ex: 1920x1080).
     2. En plein écran, le bandeau est masqué *avant* le calcul d'adaptation et positionné en `position: absolute` pour flotter au survol sans pousser le flux vidéo.

5. **Maintien du Stretch à 100% en fenêtre agrandie/redimensionnée** :
   - *Comportement ajusté* : En mode fenêtre agrandie ou maximisée, le mode Stretch étire bien l'image sur 100% de la largeur et 100% de la hauteur du corps de la fenêtre (`w = bw`, `h = bh`, `foreground-size: 100% 100%`).
   - Le plein écran conserve tous ses réglages d'origine sans aucune altération.
   - Les clics souris et positions du curseur restent pixel-perfect en fenêtre agrandie grâce au ratio d'échelle dédié `cursor_scale_x` et `cursor_scale_y`.

---

## 2. Fichiers Modifiés

- `rustdesk/src/ui/remote.rs` :
  - Exposition de `change_resolution(i32, i32, i32)` dans la macro `dispatch_script_call!` et implémentation dans `SciterSession`.
- `rustdesk/src/ui/remote.tis` :
  - Découplage de l'échelle du curseur (`cursor_scale_x`, `cursor_scale_y`).
  - Révision de `adaptDisplay()` pour appliquer l'étirement total en mode stretch.
  - Ajout de la fonction `adjustRemoteResolution(force)`.
  - Adaptation de `onMouse`, `setCursorPosition`, `updateCursor`.
- `rustdesk/src/ui/remote.css` :
  - Ajout de la règle CSS `video#handler.stretch { foreground-size: 100% 100%; }`.
- `rustdesk/src/ui/header.tis` :
  - Ajout de l'option de menu `#fit-local-resolution` ("Adapter à la résolution locale").
  - Déclenchement de `adjustRemoteResolution()` lors de la sélection du mode Stretch.

---

## 3. Procédure de Retour Arrière (Rollback)

Une branche de sauvegarde dédiée a été créée avant d'appliquer ces modifications :
**`backup/pre-stretch-fix`** (commit `fb79ea0e0`).

### Option A : Restaurer uniquement les fichiers UI de la vue technicien
Si vous souhaitez annuler uniquement les modifications de la vue technicien tout en conservant le reste de votre travail :

```powershell
# Depuis le dossier rustdesk
cd c:\Users\Administrator\Documents\Projets\projet\rustdesk

# Restaurer les 4 fichiers sources UI
git checkout backup/pre-stretch-fix -- src/ui/remote.rs src/ui/remote.tis src/ui/remote.css src/ui/header.tis

# Ré-inliner les ressources Sciter
python res\inline-sciter.py

# Recompiler RustDesk
cargo build --locked --release --features inline
```

### Option B : Revenir complètement à l'état sauvegardé
Si vous souhaitez réinitialiser l'ensemble du dépôt RustDesk à la sauvegarde exacte :

```powershell
cd c:\Users\Administrator\Documents\Projets\projet\rustdesk
git reset --hard backup/pre-stretch-fix
python res\inline-sciter.py
cargo build --locked --release --features inline
```
