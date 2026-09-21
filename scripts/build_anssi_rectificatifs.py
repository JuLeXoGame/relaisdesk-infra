"""Generate unsigned corrections; never modify the signed September 5 originals."""
from pathlib import Path
from reportlab.lib import colors
from reportlab.lib.enums import TA_LEFT
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import mm
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer, PageBreak, KeepTogether

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "output" / "pdf" / "anssi-rectificatifs-2026-09-07"
OUT.mkdir(parents=True, exist_ok=True)
pdfmetrics.registerFont(TTFont("Legal", "C:/Windows/Fonts/arial.ttf"))
pdfmetrics.registerFont(TTFont("LegalBold", "C:/Windows/Fonts/arialbd.ttf"))
pdfmetrics.registerFontFamily("Legal", normal="Legal", bold="LegalBold")
styles = getSampleStyleSheet()
styles.add(ParagraphStyle(name="TextLegal", fontName="Legal", fontSize=10, leading=14, spaceAfter=8))
styles.add(ParagraphStyle(name="TitleLegal", fontName="LegalBold", fontSize=20, leading=24, textColor=colors.HexColor("#16324f"), spaceAfter=13))
styles.add(ParagraphStyle(name="HeadingLegal", fontName="LegalBold", fontSize=12, leading=16, spaceBefore=12, spaceAfter=6, keepWithNext=True))
styles.add(ParagraphStyle(name="SmallLegal", fontName="Legal", fontSize=8, leading=11, textColor=colors.HexColor("#46556a"), spaceAfter=7))

CLIENT = "72d1d638e56d6f2dd4f357263000738144633660"
SERVER = "bd5f43fe2976ce34dd7b1ce850ea00f4ec6e5bd9"
IDENTITY = "Julien BELLOT, entrepreneur individuel (EI) - SIREN 940 747 108 - SIRET 940 747 108 00014. 2 Chemin de Lonzais, 03170 Bizeneuille, France. Téléphone : 06 62 85 59 30. Courriel : contact@relaisdesk.fr."


def para(text, style="TextLegal"):
    return Paragraph(text, styles[style])


def section(title, *paragraphs):
    return [para(title, "HeadingLegal")] + [para(p) for p in paragraphs]


def page(canvas, doc):
    canvas.saveState()
    canvas.setFont("Legal", 8)
    canvas.setFillColor(colors.HexColor("#46556a"))
    canvas.drawString(20 * mm, 285 * mm, "RELAISDESK  |  COMPLEMENT RECTIFICATIF  |  7 SEPTEMBRE 2026")
    canvas.line(20 * mm, 281 * mm, 190 * mm, 281 * mm)
    canvas.drawString(20 * mm, 12 * mm, "Document préparé pour validation du déclarant - aucune signature reproduite")
    canvas.drawRightString(190 * mm, 12 * mm, str(doc.page))
    canvas.restoreState()


def build(name, title, story):
    document = SimpleDocTemplate(str(OUT / name), pagesize=(210 * mm, 297 * mm),
                                 rightMargin=20 * mm, leftMargin=20 * mm,
                                 topMargin=25 * mm, bottomMargin=23 * mm,
                                 title=title, author="Julien BELLOT EI / RelaisDesk")
    document.build([para(title, "TitleLegal")] + story, onFirstPage=page, onLaterPages=page)


addendum = [para("Complément au formulaire et aux pièces datés du 5 septembre 2026, déjà transmis à l'ANSSI. Ce document identifie les corrections ; il ne constitue ni une nouvelle attestation délivrée par l'ANSSI, ni une certification de sécurité.")]
addendum += section("1. Identification du dossier", IDENTITY,
                    "Référence ANSSI / date du courriel initial : ........................................................................<br/>A compléter par le déclarant avant envoi. Produit : RelaisDesk, client et serveurs de téléassistance.")
addendum += section("2. Cadre A - Qualité du déclarant",
                    "L'exploitant est une personne physique exerçant en nom propre sous le statut d'entrepreneur individuel, et non une société ou une personne morale distincte. La mention portée au cadre A.1 du formulaire initial doit être lue avec cette rectification. L'identité, le SIRET et les coordonnées restent ceux indiqués ci-dessus.",
                    "L'avis de situation SIRENE déjà joint identifie l'établissement mais n'est pas présenté comme un extrait Kbis équivalent. Un justificatif d'immatriculation RNE récent, ou le document demandé par le bureau instructeur pour cette EI, sera fourni en complément.")
addendum += section("3. Cadre B.1 - Versions identifiées",
                    "Client RustDesk modifié : version du paquet source 1.4.9, commit <font size=8>" + CLIENT + "</font>.<br/>Serveurs hbbs et hbbr : version 1.1.17, commit <font size=8>" + SERVER + "</font>.",
                    "La mention commune « Client et Serveur 1.4.x » est remplacée par cette distinction. La version commerciale des enveloppes de distribution (1.0.0) ne remplace pas les références des composants. Les mises à jour ultérieures doivent être tracées séparément.")
addendum += section("4. Cadres B.3.1 et B.3.4 - Cryptographie",
                    "Le chiffrement symétrique de session examiné repose sur libsodium crypto_secretbox / sodiumoxide secretbox : XSalsa20-Poly1305, clé de 256 bits et nonce de 192 bits. Il ne doit pas être décrit comme ChaCha20-Poly1305 avec nonce de 96 bits.",
                    "La protection de l'échange de clé symétrique utilise libsodium crypto_box (Curve25519, XSalsa20-Poly1305). Les signatures d'identité et d'autorisation utilisent Ed25519. L'accès HTTPS à l'API doit être distingué du protocole TCP/UDP RustDesk ; tous les canaux de signalisation ne sont pas assimilés à TLS 1.3. Aucune garantie générale de forward secrecy n'est avancée sans analyse dédiée de l'ensemble du protocole.")
addendum += [PageBreak()]
addendum += section("5. Cadre C - Complément à la demande de classement grand public",
                    "La demande est maintenue pour appréciation par l'ANSSI ; elle n'est pas présentée comme un classement acquis. Les logiciels sont téléchargeables en ligne ; l'accès à l'infrastructure est commercialisé par capacités simultanées. A ce jour, les nouvelles ventes sont réservées aux professionnels et le lancement B2C est différé. La présentation initiale d'une vente déjà ouverte sans restriction aux particuliers doit être corrigée.",
                    "Les interfaces des applications distribuées ne proposent pas de choix arbitraire des primitives ou de leurs tailles de clés. Toutefois les forks sont libres sous AGPLv3 : un utilisateur disposant des compétences nécessaires peut modifier les sources et recompiler. La formulation absolue selon laquelle l'utilisateur ne peut jamais modifier la cryptographie est retirée.",
                    "Complément au champ initialement vide sur l'installation : l'utilisateur télécharge et lance le Viewer ou l'application Technicien, installe si nécessaire, puis saisit les identifiants de licence ou le code temporaire. L'établissement des clés de session ne nécessite pas d'intervention cryptographique manuelle du fournisseur. Les droits système et une connectivité compatibles restent requis.")
addendum += section("6. Hébergement, pièces et portée",
                    "Le site est hébergé chez OVHcloud. L'API, la base et les serveurs RustDesk utilisent la région commerciale Oracle Cloud Infrastructure France Sud (Marseille), eu-marseille-1. Cette région existe mais est distincte d'Oracle EU Sovereign Cloud. La localisation ne vaut ni certification ni preuve autonome de conformité RGPD.",
                    "Pièces corrigées jointes : note technique, brochure commerciale et présentation de l'entreprise datées du 7 septembre 2026. Les originaux signés du 5 septembre ne sont ni effacés ni retouchés. Merci de rattacher ces pièces au dossier initial et d'indiquer si un formulaire officiel intégralement ressaisi est nécessaire.")
addendum += section("7. Validation par le déclarant",
                    "Après relecture et vérification des références, le déclarant confirme les rectifications ci-dessus et les informations de déploiement qu'il communique. La présente préparation ne remplace pas sa validation personnelle.",
                    "Fait à : ........................................  Le : ........................................<br/><br/>Nom et qualité : Julien BELLOT, entrepreneur individuel<br/><br/>Signature du déclarant :<br/><br/><br/>........................................................")
build("01_Addendum_formulaire_ANSSI_2026-09-07.pdf", "Addendum au dossier ANSSI", addendum)

technical = [para("Note technique rectificative - remplace la note technique datée du 5 septembre 2026 pour les références ci-dessous."), para(IDENTITY, "SmallLegal")]
technical += section("1. Périmètre vérifié et versions",
                     "RelaisDesk est une adaptation commerciale de RustDesk Community, avec contrôle d'autorisation. Client source : 1.4.9 ; serveurs hbbs/hbbr : 1.1.17. Les applications commerciales peuvent porter une numérotation distincte.",
                     "Client : <font size=8>" + CLIENT + "</font><br/>Serveur : <font size=8>" + SERVER + "</font>.<br/>Dépôts publics : github.com/JuLeXoGame/relaisdesk et github.com/JuLeXoGame/rustdesk-server. Les sous-modules modifiés sont également publiés.")
technical += section("2. Architecture et données",
                     "hbbs assure le rendez-vous et hbbr le relais de secours. Une session peut utiliser un lien direct entre les postes ou un relais selon le réseau. Le relais transporte les paquets de session chiffrés sans disposer, dans ce fonctionnement, de leur clé de déchiffrement. Le protocole RustDesk TCP/UDP n'est pas assimilé globalement à HTTPS.",
                     "L'API Go gère notamment licences, autorisations, commandes et fiches d'intervention. Les fiches et métadonnées sont des traitements distincts : elles ne bénéficient pas du chiffrement de bout en bout des flux d'écran. L'absence d'enregistrement de ces flux par le relais n'est pas une absence de tout traitement de données personnelles.")
technical += section("3. Primitives confirmées par le code",
                     "<b>Flux de session - XSalsa20-Poly1305 :</b> libsodium crypto_secretbox, exposé par sodiumoxide::crypto::secretbox. Clé symétrique : 32 octets (256 bits). Nonce : 24 octets (192 bits). Authentification Poly1305. L'implémentation de transport gère les nonces de messages.",
                     "<b>Protection de la clé symétrique - crypto_box :</b> Curve25519 / XSalsa20-Poly1305 via sodiumoxide::crypto::box_. La fonction create_symmetric_key_msg crée une paire de clés et une clé symétrique aléatoires, puis protège cette dernière pour le destinataire. Clés publiques et secrètes crypto_box : 32 octets chacune.",
                     "<b>Signatures - Ed25519 :</b> identité et jetons d'autorisation ; clé publique 32 octets, signature 64 octets. Les clés privées d'identité, d'autorisation réseau et de signature de manifeste ont des rôles distincts et ne doivent pas être confondues.",
                     "<b>Intégrité des versions - SHA-256 et Ed25519 :</b> empreinte des artefacts et signature du manifeste. Cette signature n'est pas une signature Windows Authenticode.")
technical += [PageBreak()]
technical += section("4. Transport HTTPS et limites de description",
                     "L'API de production est publiée en HTTPS, avec terminaison TLS et configuration dépendant du déploiement. Les versions TLS et suites effectivement négociées doivent être relevées sur l'instance concernée. Une négociation TLS susceptible d'utiliser ChaCha20 ou AES-GCM ne change pas l'algorithme secretbox du flux RustDesk.",
                     "La présente note ne revendique ni TLS 1.3 sur tous les flux, ni nonce secretbox de 96 bits, ni chiffrement inviolable, ni certification ANSSI. Elle ne conclut pas à une garantie de confidentialité persistante après compromission de toutes les catégories de clés (forward secrecy) sans analyse spécifique. Le comportement des modes de repli et la configuration d'authentification font partie de la recette de sécurité de chaque version.")
technical += section("5. Gestion des clés et autorisation",
                     "La génération des clés de session utilise les fonctions cryptographiques de libsodium. Le stockage et les droits des clés persistantes dépendent du poste et du déploiement ; aucune affirmation globale de chiffrement au repos de toutes les clés privées n'est faite. Les clés privées ne doivent pas être publiées dans le code, les paquets ou les journaux.",
                     "Le fork ajoute des jetons courts signés Ed25519, des preuves de possession, une protection anti-rejeu et le contrôle de capacité. Le mode obligatoire est activé par la configuration serveur prévue à cet effet. L'ouverture du code n'accorde pas une autorisation d'accès à l'infrastructure commerciale.")
technical += section("6. Distribution et hébergement",
                     "Les sources du fork sont sous AGPLv3 et peuvent être modifiées puis recompilées. Les paquets Windows/Linux sont proposés depuis relaisdesk.fr. Les nouvelles ventes B2C restent différées. Le classement « grand public » est demandé à l'ANSSI et reste soumis à son appréciation.",
                     "Site chez OVHcloud ; API, base et RustDesk dans Oracle Cloud Infrastructure France Sud (Marseille), eu-marseille-1, région commerciale distincte d'Oracle EU Sovereign Cloud. Cette note ne vaut pas preuve contractuelle des transferts internationaux des fournisseurs.")
technical += section("7. Références techniques",
                     "Code : libs/hbb_common/src/tcp.rs (secretbox, Encrypt, decode), src/common.rs (create_symmetric_key_msg), Cargo.toml et sous-modules aux commits indiqués. Références publiques : libsodium.gitbook.io/doc/secret-key_cryptography/secretbox ; libsodium.gitbook.io/doc/public-key_cryptography/authenticated_encryption ; docs.rs/sodiumoxide.")
technical += [para("Validation du déclarant : lieu et date ........................................<br/><br/>Signature : ........................................................")]
build("02_Note_technique_rectifiee_2026-09-07.pdf", "Note technique rectificative", technical)

commercial = [para("Présentation commerciale rectifiée - situation au 7 septembre 2026."), para(IDENTITY, "SmallLegal")]
commercial += section("Assistance à distance pour les professionnels",
                      "RelaisDesk propose une infrastructure d'accès distant basée sur un fork indépendant de RustDesk Community. Les applications Technicien et Viewer permettent la mise en relation, la prise en main et le transfert de fichiers selon les capacités du logiciel et les autorisations données par la personne assistée.")
commercial += section("Offres et capacités simultanées",
                      "<b>Starter :</b> 24,90 € pour 30 jours ou 239,00 € pour 365 jours ; 1 technicien simultané.<br/><b>Pro :</b> 129,00 € pour 30 jours ou 1 290,00 € pour 365 jours ; jusqu'à 5 techniciens simultanés.<br/><b>Personnalisé :</b> 10 à 500 techniciens ; à partir de 239,00 € pour 30 jours ou 2 390,00 € pour 365 jours pour 10 techniciens. Montant de la capacité choisie affiché avant commande.",
                      "Montants nets à payer. TVA non applicable, art. 293 B du CGI. Le tarif, la capacité et la période validés dans la commande font foi. Paiement en une fois, sans reconduction tacite ; renouvellement à valider expressément. Les nouvelles ventes sont réservées aux professionnels ; le B2C n'est pas encore ouvert.")
commercial += section("Installation et protection",
                      "L'installation sur plusieurs ordinateurs n'est pas limitée par l'offre : seuls les postes techniciens connectés simultanément consomment sa capacité. Le Viewer est proposé sans surcoût par session dans les limites techniques du service.",
                      "Les sessions utilisent le chiffrement du protocole RustDesk ; les métadonnées et fiches conservées par l'API sont distinctes des flux d'écran chiffrés de bout en bout. Site OVHcloud et backend Oracle Marseille eu-marseille-1 (région commerciale). Aucune certification, souveraineté Oracle EU Sovereign Cloud, bande passante dédiée ou sécurité absolue n'est promise par cette brochure.")
commercial += section("Informations et code source",
                      "Service et conditions : https://relaisdesk.fr<br/>Sources et licences : https://relaisdesk.fr/logiciel-libre.html<br/>RelaisDesk est indépendant de RustDesk ; les droits AGPLv3 restent applicables. La distribution n'est pas conditionnée à une signature SignPath.")
build("03_Brochure_commerciale_rectifiee_2026-09-07.pdf", "RelaisDesk - Offre de service", commercial)

company = [para("Présentation rectificative de l'exploitant - 7 septembre 2026.")]
company += section("Identité", IDENTITY,
                   "Forme juridique : entrepreneur individuel (personne physique), et non SAS.<br/>Enseigne historique : Informatique A Domicile 03.<br/>RelaisDesk : nom utilisé pour l'offre d'assistance à distance.<br/>Code APE de l'établissement : 9511Z.<br/>TVA non applicable, art. 293 B du CGI.")
company += section("Activité et offre",
                   "Julien BELLOT exploite une activité de services informatiques. L'offre RelaisDesk fournit l'accès à une infrastructure de téléassistance et des applications issues de RustDesk Community. Le service commercial, les composants libres et leurs droits sont distingués dans les CGV et les notices de licence.",
                   "Les renseignements d'immatriculation et l'éventuelle déclaration d'une activité complémentaire sont justifiés par les documents du registre compétent. L'avis SIRENE n'est pas présenté comme un Kbis ni comme une preuve suffisante de toutes les formalités professionnelles.")
company += section("Organisation technique et données",
                   "Le site demeure hébergé chez OVHcloud. Le backend repose sur Oracle Cloud Infrastructure dans la région commerciale France Sud (Marseille), eu-marseille-1. Cette localisation ne désigne pas l'offre Oracle EU Sovereign Cloud.",
                   "L'exploitant gère les comptes, paiements et moyens techniques du service. La politique de confidentialité décrit les traitements ; une annexe RGPD encadre les données confiées par les clients professionnels. La localisation des serveurs, le chiffrement et l'ouverture des sources ne constituent pas à eux seuls une garantie globale de conformité ni une certification.")
company += section("Relations réglementaires",
                   "Un dossier de cryptologie a déjà été adressé à l'ANSSI. Le présent complément rectifie les informations signalées dans l'addendum. Il ne préjuge ni de sa recevabilité, ni du classement demandé, ni d'une décision de l'Agence. Les ventes B2C demeurent différées jusqu'à la préparation des conditions nécessaires.")
build("04_Presentation_EI_rectifiee_2026-09-07.pdf", "Présentation de l'entreprise", company)

print("Created 4 unsigned PDF documents in", OUT)
