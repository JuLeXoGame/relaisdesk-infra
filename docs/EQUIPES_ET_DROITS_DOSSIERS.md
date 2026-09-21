# Equipes Pro et droits par dossier — travaux en cours

Demande du 12 septembre 2026 : gestion legere dans l'espace client existant,
invitations par e-mail et utilisateurs techniciens limites aux dossiers autorises.
Ce document est un suivi d'implementation, pas une annonce de disponibilite.

## Regles retenues

- Pro, Ultra et offres personnalisees eligibles ; Starter sans equipe.
- Maximum d'utilisateurs invites/actifs egal au nombre de techniciens achete.
  Le proprietaire garde son compte d'administration commercial. Le quota reseau
  de postes techniciens simultanement connectes reste commun a toute la licence,
  proprietaire compris lorsqu'il utilise le Technicien.
- Une invitation en attente reserve une place jusqu'a expiration. Liens a usage
  unique, expires et revocables ; aucun mot de passe transmis par e-mail.
- Comptes personnels et 2FA existants reutilises ; aucune cle de licence du
  proprietaire communiquee aux invites, aucun acces a ses factures.
- Autorisation sur un dossier et ses sous-dossiers. Les postes sans dossier
  restent inaccessibles aux invites. Les invites ne peuvent ni deplacer des postes
  ni modifier les dossiers ni creer d'autres utilisateurs.
- Droits verifies dans l'API ET lors de l'autorisation reseau hbbs/hbbr. Les
  anciennes versions doivent refuser les nouveaux jetons restreints, pas ignorer
  silencieusement leurs restrictions.
- Aucun nouveau service externe : SQLite, mail et processus Go existants.
- Compte de test demande : julienbellot03170@gmail.com, Pro sans date de fin,
  sans paiement ni abonnement Stripe. L'attribution reelle reste a verifier.

## Verification avant disponibilite

Schemas et migrations, authentification individuelle/2FA, invitations et quotas
concurrents, isolation interclients, controle des dossiers descendants, refus des
acces directs par identifiant non autorise, tokens reseau et quota commun,
expiration/revocation, interface administrateur/technicien et essais des programmes.

Aucun deploiement, publication de binaires ou envoi d'invitation reel n'est
atteste par ce fichier. Le site se modifie directement dans `relaisdesk/`, sans ZIP.
