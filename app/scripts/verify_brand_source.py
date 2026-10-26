"""Compare current official source bytes with the saved design audit, without a browser."""

import hashlib
import json
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path


def main():
    repository = Path(__file__).resolve().parents[2]
    evidence = repository / "docs/design/evidence"
    previous = json.loads((evidence / "website-reference.json").read_text(encoding="utf-8"))
    origin = "https://copypaste.hk"
    report = {
        "checked_at": datetime.now(timezone.utc).isoformat(),
        "method": "Windows native Python HTTPS; SHA-256 of actual response bytes",
        "previous_capture_at": previous["captured_at"],
        "browser_invoked": False,
        "scope": "Homepage, shared stylesheet/script and both currently used PNG logos; not a new browser or full-site audit",
        "resources": [],
        "errors": [],
    }
    sources = (
        "/", "/styles.css", "/app.js",
        "/assets/logo/CopyPaste_Light.png", "/assets/logo/CopyPaste_Dark.png",
    )
    for relative in sources:
        url = origin + relative
        prior = next((item for item in previous["assets"] if item["url"].rstrip("/") == url.rstrip("/")), None)
        try:
            request = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0 CopyPaste-design-review"})
            with urllib.request.urlopen(request, timeout=30) as response:
                body = response.read(2 * 1024 * 1024 + 1)
                if len(body) > 2 * 1024 * 1024:
                    raise ValueError("Source exceeds the 2 MiB audit limit")
                digest = hashlib.sha256(body).hexdigest()
                entry = {
                    "url": response.url,
                    "status": response.status,
                    "bytes": len(body),
                    "sha256": digest,
                    "previous_sha256": prior["sha256"] if prior else None,
                    "unchanged": bool(prior and prior["sha256"] == digest),
                }
                if prior and prior.get("local_file"):
                    local_asset = evidence.parent / prior["local_file"]
                    local_digest = hashlib.sha256(local_asset.read_bytes()).hexdigest()
                    entry.update({
                        "local_file": prior["local_file"],
                        "local_sha256": local_digest,
                        "local_matches_reference": local_digest == prior["sha256"],
                        "local_matches_current": local_digest == digest,
                    })
                report["resources"].append(entry)
        except (OSError, ValueError, urllib.error.URLError) as error:
            report["errors"].append({"url": url, "error": str(error)})
    report["all_sources_unchanged"] = not report["errors"] and len(report["resources"]) == len(sources) and all(item["unchanged"] for item in report["resources"])
    local_assets = [item for item in report["resources"] if "local_file" in item]
    report["all_local_logos_match"] = len(local_assets) == 2 and all(item["local_matches_reference"] and item["local_matches_current"] for item in local_assets)
    (evidence / "website-source-refresh.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False))
    return 0 if report["all_sources_unchanged"] and report["all_local_logos_match"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
