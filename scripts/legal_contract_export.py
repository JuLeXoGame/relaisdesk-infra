"""Render the accepted HTML contract + DPA as one durable text attachment.

Read-only by default. --patch emits an apply_patch document for a NEW version;
an existing archive is never silently rewritten.
"""
import argparse
import re
import sys
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urljoin

ROOT = Path(__file__).resolve().parents[1]
# Read the deployed contract's source version; never rewrite its email archive.
def current_version():
    mailer = (ROOT / "api/mailer/mailer.go").read_text(encoding="utf-8")
    match = re.search(r'^const CurrentTermsVersion = "(\d{4}-\d{2}-\d{2})"$', mailer, re.MULTILINE)
    if not match:
        raise ValueError("Version contractuelle introuvable dans le mailer")
    return match.group(1)


VERSION = current_version()


class ContractText(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.depth = 0
        self.parts = []
        self.links = []

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "div":
            if self.depth:
                self.depth += 1
            elif "legal-body" in attrs.get("class", "").split():
                self.depth = 1
        if not self.depth:
            return
        if tag in ("p", "h1", "h2", "h3", "li", "blockquote"):
            self.parts.append("\n\n" if tag != "li" else "\n- ")
        if tag == "a":
            self.links.append(attrs.get("href", ""))

    def handle_endtag(self, tag):
        if not self.depth:
            return
        if tag == "a" and self.links:
            href = self.links.pop()
            if href and not href.startswith(("mailto:", "tel:")):
                self.parts.append(" (" + urljoin("https://relaisdesk.fr/", href) + ")")
        if tag in ("p", "h1", "h2", "h3", "blockquote", "ul", "ol"):
            self.parts.append("\n\n")
        if tag == "div":
            self.depth -= 1

    def handle_data(self, data):
        if self.depth:
            self.parts.append(re.sub(r"\s+", " ", data))

    def result(self):
        text = "".join(self.parts)
        text = re.sub(r"[ \t]+\n", "\n", text)
        text = re.sub(r"\n[ \t]+", "\n", text)
        text = re.sub(r"\n{3,}", "\n\n", text)
        return text.strip()


def render_contract():
    sections = ["CONDITIONS GÉNÉRALES DE VENTE ET D'UTILISATION RELAISDESK\nVersion contractuelle " + VERSION]
    for filename, title in [("cgv.html", ""), ("sous-traitance-rgpd.html", "ANNEXE DE SOUS-TRAITANCE RGPD\nVersion " + VERSION)]:
        html = (ROOT / "relaisdesk" / filename).read_text(encoding="utf-8")
        if not re.search(r"Version\s+" + re.escape(VERSION) + r"\b", html):
            raise ValueError(filename + " : version affichée différente de l’API")
        parser = ContractText()
        parser.feed(html)
        if title:
            sections.append(title)
        sections.append(parser.result())
    return "\n\n".join(sections) + "\n"


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument("--patch", action="store_true")
    parsed = args.parse_args()
    relative = "api/mailer/legal/CGV-RelaisDesk-" + VERSION + ".txt"
    path = ROOT / relative
    expected = render_contract()
    if path.exists():
        if path.read_text(encoding="utf-8") != expected:
            raise SystemExit("Archive différente du HTML : contrôler la version et créer une nouvelle archive, sans réécrire les acceptations passées.")
        print("Contrat HTML, annexe et archive email : identiques")
    elif parsed.patch:
        print("*** Begin Patch\n*** Add File: " + relative)
        print("\n".join("+" + line for line in expected.splitlines()))
        print("*** End Patch")
    else:
        raise SystemExit("Archive absente : utiliser --patch puis appliquer le patch avant la compilation.")


if __name__ == "__main__":
    main()
