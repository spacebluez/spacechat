import base64
import json
import os
import pathlib
import subprocess
import tempfile
import unittest
import zipfile


ROOT = pathlib.Path(__file__).resolve().parents[1]


class DockerArtifactBuilderTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tools_temporary = tempfile.TemporaryDirectory()
        cls.tools = pathlib.Path(cls.tools_temporary.name)
        cls.release_tool = cls.tools / "spacechat-release"
        cls.server = cls.tools / "xchat-server"
        environment = os.environ.copy()
        environment["GOCACHE"] = "/tmp/spacechat-go-build-cache"
        subprocess.run(
            ["go", "build", "-o", cls.release_tool, "./cmd/spacechat-release"],
            cwd=ROOT,
            env=environment,
            check=True,
        )
        subprocess.run(
            ["go", "build", "-o", cls.server, "./cmd/xchat-server"],
            cwd=ROOT,
            env=environment,
            check=True,
        )

    @classmethod
    def tearDownClass(cls):
        cls.tools_temporary.cleanup()

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.temporary.name) / "path with spaces"
        self.root.mkdir()
        self.terminal = self.root / "terminal input"
        (self.terminal / "settings").mkdir(parents=True)
        (self.terminal / "WindowsTerminal.exe").write_bytes(b"terminal")
        (self.terminal / ".portable").write_text("", encoding="utf-8")
        (self.terminal / "settings/settings.json").write_text("{}\n", encoding="utf-8")

    def tearDown(self):
        self.temporary.cleanup()

    def environment(self):
        environment = os.environ.copy()
        environment.pop("SPACECHAT_TEST_FAIL_AFTER_BACKUP", None)
        environment.pop("SPACECHAT_TEST_FAIL_PUBLISH_RENAME", None)
        environment.update(
            {
                "SPACECHAT_VERSION": "0.4.0",
                "SPACECHAT_MINIMUM_VERSION": "0.0.0",
                "SPACECHAT_OUTPUT_DIR": str(self.root / "artifact output"),
                "SPACECHAT_SIGNING_KEY_PATH": str(
                    ROOT / "deploy/docker/default-update-signing.seed"
                ),
                "SPACECHAT_CLIENT_CA_PATH": str(
                    ROOT / "deploy/docker/empty-client-ca.pem"
                ),
                "SPACECHAT_TERMINAL_DIR": str(self.terminal),
                "SPACECHAT_RELEASE_BIN": str(self.release_tool),
                "SPACECHAT_SERVER_BIN": str(self.server),
                "GOCACHE": "/tmp/spacechat-go-build-cache",
            }
        )
        return environment

    def run_builder(self, environment=None, check=True):
        return subprocess.run(
            ["sh", str(ROOT / "deploy/docker/build-artifacts.sh")],
            cwd=ROOT,
            env=environment or self.environment(),
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            check=check,
        )

    def test_default_seed_is_accepted_by_the_real_release_tool(self):
        private_seed = self.root / "update-signing.seed"
        private_seed.write_bytes(
            (ROOT / "deploy/docker/default-update-signing.seed").read_bytes()
        )
        private_seed.chmod(0o600)
        result = subprocess.run(
            [
                self.release_tool,
                "public-key",
                "-private-key",
                private_seed,
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            check=True,
        )
        self.assertEqual(32, len(base64.b64decode(result.stdout.strip(), validate=True)))

    def test_builder_generates_and_atomically_replaces_a_verified_release(self):
        first = self.run_builder()
        release = self.root / "artifact output/release"
        installers = json.loads((release / "installers/installers.json").read_text())
        self.assertEqual(2, installers["schema"])
        self.assertEqual("request", installers["server_mode"])
        self.assertTrue((release / "updates/manifest.sig").is_file())
        self.assertEqual(
            32,
            len(
                base64.b64decode(
                    (release / "update-public.key").read_text().strip(), validate=True
                )
            ),
        )
        with zipfile.ZipFile(
            release / "installers/spacechat-windows-amd64-0.4.0.zip"
        ) as archive:
            self.assertEqual(
                {
                    "README.md",
                    "install.ps1",
                    "spacechat-client.exe",
                    "spacechat.exe",
                    "terminal/.portable",
                    "terminal/WindowsTerminal.exe",
                    "terminal/settings/settings.json",
                },
                {name.rstrip("/") for name in archive.namelist() if not name.endswith("/")},
            )
        with zipfile.ZipFile(
            release / "installers/spacechat-linux-amd64-0.4.0.zip"
        ) as archive:
            self.assertEqual(
                {"README.md", "install.sh", "spacechat-client", "spacechat"},
                {name.rstrip("/") for name in archive.namelist() if not name.endswith("/")},
            )
        subprocess.run(
            [
                self.server,
                "-validate-updates",
                "-update-dir",
                release / "updates",
                "-installer-dir",
                release / "installers",
                "-update-public-key-file",
                release / "update-public.key",
            ],
            check=True,
        )
        marker = release / "obsolete"
        marker.write_text("old", encoding="utf-8")
        second = self.run_builder()
        self.assertFalse(marker.exists())
        self.assertFalse((self.root / "artifact output/release.previous").exists())
        self.assertEqual([], list((self.root / "artifact output").glob(".release.*")))
        self.assertTrue((self.root / "artifact output").stat().st_mode & 0o001)
        seed = (ROOT / "deploy/docker/default-update-signing.seed").read_text().strip()
        self.assertNotIn(seed, first.stdout + first.stderr + second.stdout + second.stderr)
        for path in release.rglob("*"):
            self.assertTrue(path.stat().st_mode & 0o004, path)

    def test_failed_final_validation_preserves_the_current_release(self):
        self.run_builder()
        output = self.root / "artifact output"
        marker = output / "release/keep-me"
        marker.write_text("current", encoding="utf-8")
        rejecting_server = self.root / "reject updates"
        rejecting_server.write_text("#!/bin/sh\nexit 1\n", encoding="utf-8")
        rejecting_server.chmod(0o755)
        environment = self.environment()
        environment["SPACECHAT_SERVER_BIN"] = str(rejecting_server)

        failed = self.run_builder(environment, check=False)

        self.assertNotEqual(0, failed.returncode)
        self.assertEqual("current", marker.read_text(encoding="utf-8"))
        self.assertFalse((output / "release.previous").exists())
        self.assertEqual([], list(output.glob(".release.*")))

    def test_term_after_backup_restores_release_and_allows_retry(self):
        self.run_builder()
        output = self.root / "artifact output"
        marker = output / "release/keep-me"
        marker.write_text("current", encoding="utf-8")
        interrupted_environment = self.environment()
        interrupted_environment["SPACECHAT_TEST_FAIL_AFTER_BACKUP"] = "TERM"

        interrupted = self.run_builder(interrupted_environment, check=False)

        self.assertNotEqual(0, interrupted.returncode)
        self.assertEqual("current", marker.read_text(encoding="utf-8"))
        self.assertFalse((output / "release.previous").exists())
        self.assertEqual([], list(output.glob(".release.*")))
        self.run_builder()
        self.assertFalse(marker.exists())
        self.assertFalse((output / "release.previous").exists())
        self.assertEqual([], list(output.glob(".release.*")))

    def test_failed_publish_rename_restores_release_and_allows_retry(self):
        self.run_builder()
        output = self.root / "artifact output"
        marker = output / "release/keep-me"
        marker.write_text("current", encoding="utf-8")
        failed_environment = self.environment()
        failed_environment["SPACECHAT_TEST_FAIL_PUBLISH_RENAME"] = "1"

        failed = self.run_builder(failed_environment, check=False)

        self.assertNotEqual(0, failed.returncode)
        self.assertEqual("current", marker.read_text(encoding="utf-8"))
        self.assertFalse((output / "release.previous").exists())
        self.assertEqual([], list(output.glob(".release.*")))
        self.run_builder()
        self.assertFalse(marker.exists())
        self.assertFalse((output / "release.previous").exists())
        self.assertEqual([], list(output.glob(".release.*")))

    def test_next_run_recovers_an_orphaned_previous_release(self):
        self.run_builder()
        output = self.root / "artifact output"
        marker = output / "release/keep-me"
        marker.write_text("current", encoding="utf-8")
        (output / "release").rename(output / "release.previous")
        failed_environment = self.environment()
        failed_environment["SPACECHAT_SIGNING_KEY_PATH"] = str(
            self.root / "missing-signing-key"
        )

        failed = self.run_builder(failed_environment, check=False)

        self.assertNotEqual(0, failed.returncode)
        self.assertEqual("current", marker.read_text(encoding="utf-8"))
        self.assertFalse((output / "release.previous").exists())
        self.assertEqual([], list(output.glob(".release.*")))
        self.run_builder()
        self.assertFalse(marker.exists())

    def test_permission_normalization_failure_preserves_current_release(self):
        self.run_builder()
        output = self.root / "artifact output"
        marker = output / "release/keep-me"
        marker.write_text("current", encoding="utf-8")
        failing_bin = self.root / "failing bin"
        failing_bin.mkdir()
        failing_chmod = failing_bin / "chmod"
        failing_chmod.write_text("#!/bin/sh\nexit 1\n", encoding="utf-8")
        failing_chmod.chmod(0o755)
        failed_environment = self.environment()
        failed_environment["PATH"] = str(failing_bin) + os.pathsep + os.environ["PATH"]

        failed = self.run_builder(failed_environment, check=False)

        self.assertNotEqual(0, failed.returncode)
        self.assertEqual("current", marker.read_text(encoding="utf-8"))
        self.assertFalse((output / "release.previous").exists())
        self.assertEqual([], list(output.glob(".release.*")))

    def test_rejects_invalid_version_and_client_ca_before_replacing_release(self):
        output = self.root / "artifact output"
        release = output / "release"
        release.mkdir(parents=True)
        marker = release / "keep-me"
        marker.write_text("current", encoding="utf-8")

        invalid_version = self.environment()
        invalid_version["SPACECHAT_VERSION"] = "01.2.3"
        self.assertNotEqual(0, self.run_builder(invalid_version, check=False).returncode)

        private_ca = self.root / "private-ca.pem"
        private_ca.write_text(
            "-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----\n"
            "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----\n",
            encoding="utf-8",
        )
        invalid_ca = self.environment()
        invalid_ca["SPACECHAT_CLIENT_CA_PATH"] = str(private_ca)
        result = self.run_builder(invalid_ca, check=False)

        self.assertNotEqual(0, result.returncode)
        self.assertEqual("current", marker.read_text(encoding="utf-8"))
        self.assertNotIn("secret", result.stdout + result.stderr)
        self.assertEqual([], list(output.glob(".release.*")))

    def test_rejects_invalid_certificate_without_a_private_key_marker(self):
        output = self.root / "artifact output"
        release = output / "release"
        release.mkdir(parents=True)
        marker = release / "keep-me"
        marker.write_text("current", encoding="utf-8")
        invalid_certificate = self.root / "invalid-certificate.pem"
        invalid_certificate.write_text(
            "-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----\n",
            encoding="utf-8",
        )
        environment = self.environment()
        environment["SPACECHAT_CLIENT_CA_PATH"] = str(invalid_certificate)

        result = self.run_builder(environment, check=False)

        self.assertNotEqual(0, result.returncode)
        self.assertEqual("current", marker.read_text(encoding="utf-8"))
        self.assertFalse((output / "release.previous").exists())
        self.assertEqual([], list(output.glob(".release.*")))


if __name__ == "__main__":
    unittest.main()
