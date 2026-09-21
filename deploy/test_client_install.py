import json
import os
import pathlib
import subprocess
import tempfile
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]


class ClientInstallerTests(unittest.TestCase):
    def test_windows_installs_per_user_and_updates_only_user_path(self):
        source = (ROOT / "deploy" / "client" / "install.ps1").read_text(encoding="utf-8")
        for expected in (
            "$env:LOCALAPPDATA",
            '"SpaceChat"',
            '"bin"',
            '"versions"',
            '"config.json"',
            '"current"',
            "ConvertTo-Json -Compress",
            'SetEnvironmentVariable("Path"',
            '"User")',
            "--self-check",
            '"spacechat $Version"',
            "Move-Item",
        ):
            self.assertIn(expected, source)
        self.assertNotIn('"Machine")', source)
        self.assertIn("wss?://", source)

    def test_linux_uses_xdg_layout_and_atomic_pointer_files(self):
        source = (ROOT / "deploy" / "client" / "install.sh").read_text(encoding="utf-8")
        for expected in (
            "${XDG_CONFIG_HOME:-$HOME/.config}",
            "${XDG_DATA_HOME:-$HOME/.local/share}",
            "$HOME/.local/bin",
            "config_dir=$config_root/spacechat",
            '"$config_dir/config.json"',
            '"$install_root/current"',
            "target_client=$target/spacechat-client",
            "--self-check",
            '"spacechat $version"',
            "mv -f",
            "wss://*|ws://*",
        ):
            self.assertIn(expected, source)
        self.assertIn("$HOME/.profile", source)

    def test_installers_require_server_and_do_not_embed_one(self):
        windows = (ROOT / "deploy" / "client" / "install.ps1").read_text(encoding="utf-8")
        linux = (ROOT / "deploy" / "client" / "install.sh").read_text(encoding="utf-8")
        self.assertIn("Mandatory", windows)
        self.assertIn('if [ "$#" -lt 2 ]', linux)
        for source in (windows, linux):
            self.assertNotIn("127.0.0.1", source)
            self.assertNotIn("localhost", source)
            self.assertNotIn("example.com", source)

    def test_linux_installer_creates_a_runnable_managed_layout(self):
        with tempfile.TemporaryDirectory() as temporary:
            temporary = pathlib.Path(temporary)
            package = temporary / "package"
            home = temporary / "home"
            package.mkdir()
            home.mkdir()
            launcher = package / "spacechat"
            client = package / "spacechat-client"
            launcher.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
            client.write_text(
                '#!/bin/sh\ncase "$1" in --version) echo "spacechat 1.2.3" ;; --self-check) exit 0 ;; *) exit 1 ;; esac\n',
                encoding="utf-8",
            )
            launcher.chmod(0o755)
            client.chmod(0o755)
            environment = os.environ.copy()
            environment.update(
                {
                    "HOME": str(home),
                    "XDG_CONFIG_HOME": str(temporary / "config"),
                    "XDG_DATA_HOME": str(temporary / "data"),
                }
            )
            result = subprocess.run(
                [
                    "sh",
                    str(ROOT / "deploy" / "client" / "install.sh"),
                    "wss://chat.invalid/ws",
                    "1.2.3",
                    str(package),
                ],
                env=environment,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                check=False,
            )
            self.assertEqual(0, result.returncode, result.stderr)
            config = temporary / "config" / "spacechat" / "config.json"
            current = temporary / "data" / "spacechat" / "current"
            installed_client = temporary / "data" / "spacechat" / "versions" / "1.2.3" / "spacechat-client"
            self.assertEqual({"server": "wss://chat.invalid/ws"}, json.loads(config.read_text(encoding="utf-8")))
            self.assertEqual("1.2.3\n", current.read_text(encoding="ascii"))
            self.assertTrue(os.access(installed_client, os.X_OK))
            self.assertTrue(os.access(home / ".local" / "bin" / "spacechat", os.X_OK))

    def test_linux_installer_rejects_package_version_mismatch(self):
        with tempfile.TemporaryDirectory() as temporary:
            temporary = pathlib.Path(temporary)
            package = temporary / "package"
            home = temporary / "home"
            package.mkdir()
            home.mkdir()
            (package / "spacechat").write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
            (package / "spacechat-client").write_text(
                '#!/bin/sh\ncase "$1" in --version) echo "spacechat 9.9.9" ;; --self-check) exit 0 ;; esac\n',
                encoding="utf-8",
            )
            (package / "spacechat").chmod(0o755)
            (package / "spacechat-client").chmod(0o755)
            environment = os.environ.copy()
            environment.update({"HOME": str(home), "XDG_CONFIG_HOME": str(temporary / "config"), "XDG_DATA_HOME": str(temporary / "data")})
            result = subprocess.run(
                ["sh", str(ROOT / "deploy" / "client" / "install.sh"), "ws://chat.invalid/ws", "1.2.3", str(package)],
                env=environment,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                check=False,
            )
            self.assertNotEqual(0, result.returncode)
            self.assertFalse((temporary / "data" / "spacechat" / "current").exists())

    def test_linux_installer_updates_existing_bash_login_profile(self):
        with tempfile.TemporaryDirectory() as temporary:
            temporary = pathlib.Path(temporary)
            package = temporary / "package"
            home = temporary / "home"
            package.mkdir()
            home.mkdir()
            (package / "spacechat").write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
            (package / "spacechat-client").write_text(
                '#!/bin/sh\ncase "$1" in --version) echo "spacechat 1.2.3" ;; --self-check) exit 0 ;; *) exit 1 ;; esac\n',
                encoding="utf-8",
            )
            (package / "spacechat").chmod(0o755)
            (package / "spacechat-client").chmod(0o755)
            bash_profile = home / ".bash_profile"
            bash_profile.write_text("export EXISTING=value\n", encoding="utf-8")
            environment = os.environ.copy()
            environment.update({"HOME": str(home), "XDG_CONFIG_HOME": str(temporary / "config"), "XDG_DATA_HOME": str(temporary / "data")})
            result = subprocess.run(
                ["sh", str(ROOT / "deploy" / "client" / "install.sh"), "ws://chat.invalid/ws", "1.2.3", str(package)],
                env=environment,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                check=False,
            )
            self.assertEqual(0, result.returncode, result.stderr)
            self.assertIn("export EXISTING=value", bash_profile.read_text(encoding="utf-8"))
            self.assertIn('export PATH="$HOME/.local/bin:$PATH"', bash_profile.read_text(encoding="utf-8"))


class ClientBuildScriptTests(unittest.TestCase):
    def test_builds_require_signing_and_emit_both_platforms(self):
        for name in ("build.ps1", "build-client.ps1"):
            source = (ROOT / "scripts" / name).read_text(encoding="utf-8")
            self.assertIn("Mandatory", source)
            self.assertIn("$SigningKey", source)
            self.assertIn('[string]$Version = "0.4.0"', source)
            self.assertIn('[string]$MinimumVersion = "0.0.0"', source)
            self.assertIn("main.updatePublicKey=$publicKey", source)
            self.assertIn("./cmd/spacechat", source)
            self.assertIn("spacechat-client-windows-amd64", source)
            self.assertIn("spacechat-client-linux-amd64", source)
            self.assertIn("spacechat-release manifest", source)
            self.assertIn("spacechat-release installers", source)
            self.assertIn("installers-$Version", source)


if __name__ == "__main__":
    unittest.main()
