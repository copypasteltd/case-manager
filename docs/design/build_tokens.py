"""Generate the CSS design-token adapter from tokens.json using native Python.

Run: python docs/design/build_tokens.py
React Native must read flat theme values from JSON rather than CSS variables.
"""
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent


def kebab(value):
    value = re.sub(r"([a-z0-9])([A-Z])", r"\1-\2", value)
    return value.replace(".", "-").lower()


def build(tokens):
    light = tokens["themes"]["light"]
    dark = tokens["themes"]["dark"]
    if set(light) != set(dark):
        raise ValueError("Light/dark token keys differ")
    lines = ["/* Generated from tokens.json; edit the JSON then regenerate. */",
             ':root, [data-theme="light"] {', '  color-scheme: light;']
    for key, value in light.items():
        lines.append(f"  --cp-{kebab(key)}: {value};")
    for key, value in tokens["fontFamilies"].items():
        lines.append(f"  --cp-font-{kebab(key)}: {value};")
    for key, values in tokens["type"].items():
        for prop, value in values.items():
            unit = "" if prop == "weight" else "px"
            lines.append(f"  --cp-type-{kebab(key)}-{kebab(prop)}: {value}{unit};")
    for value in tokens["spacing"]:
        lines.append(f"  --cp-space-{value}: {value}px;")
    for group in ("radius", "controlHeights", "tableRows", "layout"):
        for key, value in tokens[group].items():
            lines.append(f"  --cp-{kebab(group)}-{kebab(key)}: {value}px;")
    for key, value in tokens["motion"].items():
        unit = "" if key == "easing" else "ms"
        lines.append(f"  --cp-motion-{kebab(key)}: {value}{unit};")
    for key, value in tokens["zIndex"].items():
        lines.append(f"  --cp-z-{kebab(key)}: {value};")
    lines.extend(["}", '[data-theme="dark"] {', '  color-scheme: dark;'])
    for key, value in dark.items():
        lines.append(f"  --cp-{kebab(key)}: {value};")
    lines.extend(["}", ""])
    return "\n".join(lines)


if __name__ == "__main__":
    tokens = json.loads((ROOT / "tokens.json").read_text(encoding="utf-8"))
    (ROOT / "tokens.css").write_text(build(tokens), encoding="utf-8")
    print(json.dumps({"artifact": "docs/design/tokens.css", "theme_keys": len(tokens["themes"]["light"])}))
