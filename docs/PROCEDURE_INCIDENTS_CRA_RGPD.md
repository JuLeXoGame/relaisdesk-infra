# Procédure d'incident RelaisDesk - actualisée le 12 septembre 2026

Responsable : Julien BELLOT EI. Point d'entrée privé : formulaire et email
professionnel de la page contact. Ne pas demander de clé privée, mot de passe
ou contenu client dans une issue publique. Ne pas promettre un délai humain
de réponse que l'organisation n'est pas en mesure d'assurer.

## Prise en charge

1. Horodater la prise de connaissance (UTC), attribuer une référence et conserver
   les faits, versions/commits, actifs touchés et éléments de preuve minimisés.
2. Confirmer le périmètre : vulnérabilité théorique, exploitation active,
   incident grave affectant le produit, violation de données personnelles.
   Documenter les qualifications et leurs justifications, y compris en cas
   d'absence de notification. Une simple alerte de dépendance n'est pas en soi
   la preuve d'une exploitation active.
3. Contenir proportionnellement, préserver les preuves, corriger et vérifier.
   Ne pas publier de détails facilitant une exploitation avant coordination.
4. Informer les clients affectés et coordonner RustDesk / les dépendances
   lorsqu'ils sont concernés, sans divulguer de données d'autres clients.

## CRA - à partir du 11 septembre 2026

Cette échéance est désormais passée. La procédure doit être utilisable immédiatement
pour les événements entrant dans son champ ; elle n'attend ni SignPath ni une
nouvelle version de l'API. Aucun incident ou dépôt effectif n'est attesté ici.
Le contrôle documentaire restant est suivi dans
[SUIVI_JURIDIQUE_2026-09-12.md](SUIVI_JURIDIQUE_2026-09-12.md).

Faire confirmer la qualification de fabricant et le périmètre du produit
commercial RelaisDesk. La publication AGPL n'est pas une exemption automatique.
Pour les événements visés à l'article 14, préparer et transmettre via la
plateforme unique CRA les notifications au CSIRT compétent et à l'ENISA :

- alerte précoce au plus tard 24 heures après prise de connaissance ;
- notification au plus tard 72 heures après prise de connaissance ;
- rapport final de vulnérabilité activement exploitée au plus tard 14 jours
  après disponibilité d'une mesure corrective ;
- rapport final d'incident grave dans le délai prévu d'un mois après la
  notification de l'incident ; rapport d'avancement si l'incident est en cours,
  puis rapport final selon les dispositions applicables.

Consulter les instructions à jour de la plateforme au moment du signalement.
Conserver les accusés, informations communiquées et dates. Cette procédure
n'est pas une déclaration déjà effectuée ni une obligation de signaler chaque
bug à l'ANSSI. Les autres obligations générales du CRA deviennent applicables
selon leur calendrier, notamment le 11 décembre 2027 : planifier leur analyse
sans les confondre avec l'échéance de notification de septembre 2026.

Sources :
https://digital-strategy.ec.europa.eu/en/policies/cra-reporting
https://digital-strategy.ec.europa.eu/en/policies/cra-open-source
https://digital-strategy.ec.europa.eu/en/policies/cra-summary

## RGPD - procédure distincte

- Pour les données confiées comme sous-traitant : informer le Client responsable
  de traitement sans retard injustifié et l'assister, avec informations
  progressives si nécessaire. Ne pas attendre la fin de l'analyse technique.
- Pour les traitements dont RelaisDesk est responsable : examiner le risque
  pour les personnes ; notifier la CNIL lorsque requis, si possible sous
  72 heures après prise de connaissance. Motiver tout retard.
- Informer les personnes en cas de risque élevé, sauf exception applicable.
  Documenter toutes les violations et les décisions, même sans notification.
- Ne pas confondre le canal CRA, une déclaration de cryptologie et la
  notification de violation de données à la CNIL.

Source : https://www.cnil.fr/fr/reglement-europeen-protection-donnees/chapitre4

## Clôture et preuve

Conserver chronologie, décisions, notifications, correctifs, tests, versions
distribuées et leçons tirées dans un registre à accès restreint. Réviser le
risque, les accès, la documentation et les mesures de prévention. Faire un
exercice interne dès maintenant, puis après un changement majeur, sans envoyer de
fausse notification réelle. Vérifier l'accès à la
[plateforme unique ENISA et ses instructions](https://www.enisa.europa.eu/topics/product-security/single-reporting-platform-srp),
le responsable et son suppléant, ainsi que les moyens de joindre les clients.
Conserver une fiche d'exercice datée (scénario fictif, chronologie, qualification,
décision motivée, notifications préparées mais non envoyées, écarts à corriger).
