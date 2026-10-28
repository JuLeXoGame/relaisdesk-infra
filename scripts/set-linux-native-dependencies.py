"""Declare the runtime baseline of the native engine built on Ubuntu 22.04."""
import re
import sys
from pathlib import Path

control = Path(sys.argv[1])
text = control.read_text()
required = ["libc6 (>= 2.35)", "libstdc++6 (>= 11)", "libgcc-s1"]


def update(match):
    existing = [item.strip() for item in match[1].split(",")]
    existing = [item for item in existing if not re.match(r"^(libc6|libstdc\+\+6|libgcc-s1)(?:\s|$)", item)]
    return "Depends: " + ", ".join(required + existing)


text, count = re.subn(r"^Depends: ([^\n]+)$", update, text, flags=re.M)
if count != 1:
    raise RuntimeError("Expected exactly one Depends field")
control.write_text(text, newline="\n")
