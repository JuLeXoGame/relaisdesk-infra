import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Non-régression : l'explorateur de parc (arbre des dossiers + contenu
// redimensionnable) et le menu latéral repliable doivent rester câblés :
// chaque identifiant HTML attendu est référencé par le JS, chaque fonction
// clé existe, et les libellés i18n couvrent toutes les locales.
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const html = fs.readFileSync(path.join(root, 'relaisdesk/client/index.html'), 'utf8');
const js = fs.readFileSync(path.join(root, 'relaisdesk/client/app.js'), 'utf8');
const css = fs.readFileSync(path.join(root, 'relaisdesk/client/styles.css'), 'utf8');
const i18n = fs.readFileSync(path.join(root, 'relaisdesk/i18n.js'), 'utf8');

for (const id of ['fleetTree', 'fleetTreePane', 'fleetSplitter', 'fleetContentPane',
  'sidebarCollapseBtn', 'btnMoveFolder', 'moveFolderModal', 'moveFolderForm',
  'moveFolderSelect', 'moveFolderModalId', 'moveFolderSubmitBtn', 'moveFolderMessage']) {
  assert(html.includes(`id="${id}"`), `index.html : #${id} manquant`);
}
// fleetContentPane est un simple conteneur de mise en page, sans pilotage JS.
for (const id of ['fleetTree', 'fleetTreePane', 'fleetSplitter',
  'sidebarCollapseBtn', 'btnMoveFolder', 'moveFolderModal', 'moveFolderForm',
  'moveFolderSelect', 'moveFolderModalId', 'moveFolderSubmitBtn', 'moveFolderMessage']) {
  assert(js.includes(id), `app.js : référence à ${id} manquante`);
}

for (const fn of ['renderFleetTree', 'buildFleetFolderLi', 'selectFleetFolder',
  'refreshFleetView', 'confirmMoveFolder', 'openMoveFolderModal',
  'handleMoveFolderSubmit', 'handleDeleteFolder', 'isValidMoveTarget',
  'applySidebarCollapsed', 'initSidebarCollapse', 'applyFleetTreeWidth',
  'initFleetSplitter']) {
  assert(new RegExp(`function ${fn}\\b`).test(js), `app.js : fonction ${fn} manquante`);
}

// Le repli du menu ne doit pas détruire les icônes à la traduction :
// data-i18n vit sur le span interne, jamais sur le bouton.
assert(!/<button[^>]*data-i18n="nav_/.test(html), 'index.html : data-i18n direct sur .nav-item écraserait le SVG');
assert(html.includes('class="nav-label"'), 'index.html : libellés de navigation manquants');
assert(js.includes("safeCloseDialog('moveFolderModal')"), 'app.js : logout() doit fermer moveFolderModal');

for (const cls of ['.fleet-explorer', '.fleet-tree-pane', '.fleet-tree', '.fleet-node',
  '.fleet-splitter', '.fleet-content-pane', '.app.collapsed', '.nav-icon', '.nav-label']) {
  assert(css.includes(cls), `styles.css : ${cls} manquant`);
}
// Les longs libellés ne doivent ni dépasser ni élargir la sidebar.
assert(/\.nav-label\s*\{[^}]*min-width\s*:\s*0/.test(css), 'styles.css : .nav-label sans min-width:0, le texte dépasse');
assert(/\.sidebar nav\s*\{[^}]*minmax\(0,\s*1fr\)/.test(css), 'styles.css : piste de grille du menu non bornée');

// 7 locales attendues pour chaque nouvelle clé.
for (const key of ['nav_services', 'sidebar_toggle_menu', 'fleet_tree_title',
  'fleet_resize_panes', 'btn_move_folder', 'move_folder_title',
  'confirm_move_folder', 'folder_moved_success', 'move_folder_invalid_target',
  'tree_expand', 'tree_collapse']) {
  const count = (i18n.match(new RegExp(`"${key}"`, 'g')) || []).length;
  assert(count === 7, `i18n.js : "${key}" présent ${count}/7 fois`);
}

console.log('client-fleet-checks: OK');
