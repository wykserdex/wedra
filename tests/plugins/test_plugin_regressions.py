"""Live regression tests for plugin behavior that contract YAML cannot set up."""
from __future__ import annotations

import datetime
import json
import socket
import ssl
import subprocess
import sys
import tempfile
import threading
import unittest
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID


REPO = Path(__file__).resolve().parents[2]
PLUGIN_DIR = REPO / "plugins" / "community"


def run_plugin(plugin_id: str, payload: dict, *, cwd: Path | None = None):
    plugin = PLUGIN_DIR / plugin_id / "main.py"
    return subprocess.run(
        [sys.executable, str(plugin)],
        input=json.dumps(payload),
        text=True,
        capture_output=True,
        cwd=str(cwd or plugin.parent),
        timeout=15,
        check=False,
    )


class PluginRegressionTests(unittest.TestCase):
    def test_dir_lister_rejects_symlink_escape(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "workspace"
            outside = Path(tmp) / "outside"
            root.mkdir()
            outside.mkdir()
            (outside / "secret.txt").write_text("not read by plugin", encoding="utf-8")
            try:
                (root / "escape").symlink_to(outside, target_is_directory=True)
            except (OSError, NotImplementedError) as exc:
                self.skipTest(f"symlink unavailable: {exc}")

            proc = run_plugin("dir_lister", {"path": "escape"}, cwd=root)
            self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
            result = json.loads(proc.stdout)
            self.assertEqual(result["status"], "error")
            self.assertEqual(result["error"]["code"], "path_escape")

    def test_ssl_info_decodes_unverified_self_signed_certificate(self):
        key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        now = datetime.datetime.now(datetime.timezone.utc).replace(tzinfo=None)
        name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "127.0.0.1")])
        cert = (
            x509.CertificateBuilder()
            .subject_name(name)
            .issuer_name(name)
            .public_key(key.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(now - datetime.timedelta(minutes=1))
            .not_valid_after(now + datetime.timedelta(days=2))
            .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
            .sign(key, hashes.SHA256())
        )

        with tempfile.TemporaryDirectory() as tmp:
            cert_path = Path(tmp) / "cert.pem"
            key_path = Path(tmp) / "key.pem"
            cert_path.write_bytes(cert.public_bytes(serialization.Encoding.PEM))
            key_path.write_bytes(key.private_bytes(
                serialization.Encoding.PEM,
                serialization.PrivateFormat.PKCS8,
                serialization.NoEncryption(),
            ))

            server_context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            server_context.load_cert_chain(str(cert_path), str(key_path))
            listener = socket.socket()
            listener.bind(("127.0.0.1", 0))
            listener.listen(1)
            port = listener.getsockname()[1]
            server_errors: list[BaseException] = []

            def serve_once():
                try:
                    conn, _ = listener.accept()
                    with server_context.wrap_socket(conn, server_side=True):
                        pass
                except BaseException as exc:  # surfaced below for clearer failure
                    server_errors.append(exc)
                finally:
                    listener.close()

            server_thread = threading.Thread(target=serve_once, daemon=True)
            server_thread.start()
            proc = run_plugin("ssl_info", {"host": "127.0.0.1", "port": port})
            server_thread.join(timeout=10)

        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertFalse(server_thread.is_alive(), "TLS server did not finish")
        self.assertEqual(server_errors, [])
        result = json.loads(proc.stdout)
        self.assertEqual(result["status"], "ok")
        self.assertEqual(result["output"]["subject_cn"], "127.0.0.1")
        self.assertEqual(result["output"]["issuer_cn"], "127.0.0.1")
        self.assertTrue(result["output"]["self_signed"])
        self.assertEqual(result["output"]["not_after"],
                         cert.not_valid_after_utc.strftime("%Y-%m-%dT%H:%M:%S"))


if __name__ == "__main__":
    unittest.main()
