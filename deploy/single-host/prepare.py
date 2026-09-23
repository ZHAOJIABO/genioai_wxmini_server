#!/usr/bin/env python3
"""Create private first-install credentials; never overwrite existing configuration."""
import os
from pathlib import Path
import secrets


def main():
    root = Path(__file__).resolve().parent
    private = root / "private"
    if (root / ".env").exists() or private.exists():
        raise SystemExit("Existing configuration found; refusing to replace passwords.")
    template = (root / "server.yaml.example").read_text()
    providers = (root / "providers.yaml.example").read_text()
    os.umask(0o077)
    private.mkdir(mode=0o700)
    (private / "tls").mkdir(mode=0o700)
    passwords = {key: secrets.token_hex(24) for key in (
        "MYSQL_ROOT_PASSWORD", "BACKEND_DB_PASSWORD", "BRAIN_DB_PASSWORD"
    )}
    (root / ".env").write_text("RELEASE_TAG=first-install\n" + "".join(
        f"{key}={value}\n" for key, value in passwords.items()
    ))
    template = template.replace("BACKEND_PASSWORD_HERE", passwords["BACKEND_DB_PASSWORD"])
    template = template.replace("ADMIN_SECRET_HERE", secrets.token_hex(32))
    (private / "server.yaml").write_text(template)
    (private / "ai-brain.env").write_text(
        "# Fill provider credentials locally; never paste them into chat.\n"
        "GPTIMAGE_BEARER_TOKEN=\n"
        "GPTIMAGE_STORAGE_CLIENT=oss\n"
        "OSS_ENDPOINT=https://oss-cn-beijing.aliyuncs.com\n"
        "OSS_BUCKET=genioai\n"
        "OSS_PUBLIC_URL=https://genioai.oss-cn-beijing.aliyuncs.com\n"
        "OSS_ACCESS_KEY_ID=\n"
        "OSS_ACCESS_KEY_SECRET=\n"
    )
    (private / "providers.yaml").write_text(providers)
    print("Private files created. Configure business settings before starting applications.")


if __name__ == "__main__":
    main()
