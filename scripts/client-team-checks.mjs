import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Non-régression : l'explorateur des membres invités (arbre des dossiers +
// recherche, même système que le parc) doit rester câblé : chaque
// identifiant HTML attendu est référencé par le JS, chaque fonction clé
// existe, et les styles de l'arbre sont définis.
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const html = fs.readFileSync(path.join(root, 'relaisdesk/client/index.html'), 'utf8');
const js = fs.readFileSync(path.join(root, 'relaisdesk/client/teams.js'), 'utf8');
const css = fs.readFileSync(path.join(root, 'relaisdesk/client/styles.css'), 'utf8');

for (const id of ['teamSearchInput', 'teamFolderTree', 'teamMembers']) {
  assert(html.includes(`id="${id}"`), `index.html : #${id} manquant`);
  assert(js.includes(`'${id}'`) || js.includes(`"${id}"`), `teams.js : #${id} non référencé`);
}

for (const fn of ['renderTeamTree', 'selectTeamFolder', 'teamMembersIn', 'countTeamMembers', 'teamDescendantIds', 'buildTeamNodeRow']) {
  assert(js.includes(`function ${fn}(`), `teams.js : fonction ${fn} manquante`);
}

for (const cls of ['team-explorer', 'team-tree-pane', 'team-members-list', 'fleet-node', 'fleet-chevron', 'fleet-count']) {
  assert(css.includes(cls), `styles.css : classe ${cls} manquante`);
}

// Le filtre dossier + recherche doit s'appliquer à la liste des membres.
assert(js.includes('teamMembersIn(teamFolderId)'), 'teams.js : renderMembers ne filtre plus par dossier/recherche');
assert(js.includes("addEventListener('input', renderMembers)"), 'teams.js : recherche non branchée sur renderMembers');

console.log('client-team-checks: OK');
