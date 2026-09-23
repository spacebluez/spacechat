import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]


def compose_command():
    if shutil.which("docker"):
        result = subprocess.run(
            ["docker", "compose", "version"], capture_output=True, check=False
        )
        if result.returncode == 0:
            return ["docker", "compose"]
    if shutil.which("docker-compose"):
        return ["docker-compose"]
    raise unittest.SkipTest("Docker Compose v2 is required (no daemon needed)")


class DockerComposeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.compose = compose_command()

    def configuration(self, env_file=None, **overrides):
        environment = {
            key: value for key, value in os.environ.items()
            if not key.startswith(("SPACECHAT_", "COMPOSE_"))
            and key not in ("GOPROXY", "DEBIAN_MIRROR", "WINDOWS_TERMINAL_URL")
        }
        environment.update(overrides)
        with tempfile.TemporaryDirectory() as directory:
            empty_env = pathlib.Path(directory) / "empty.env"
            empty_env.touch()
            result = subprocess.run(
                self.compose + [
                    "--env-file", str(env_file or empty_env),
                    "-f", str(ROOT / "compose.yaml"), "config", "--format", "json",
                ],
                cwd=ROOT, env=environment, text=True, capture_output=True, check=True,
            )
        return json.loads(result.stdout)

    @staticmethod
    def mounts(service):
        return {mount["target"]: mount for mount in service["volumes"]}

    def test_startup_dependencies_and_persistent_volumes(self):
        config = self.configuration()
        services = config["services"]
        self.assertEqual({"artifacts", "server", "cleanup"}, set(services))
        self.assertEqual({"release-assets", "data", "admin-runtime"}, set(config["volumes"]))
        for name, service in services.items():
            self.assertEqual(name, service["build"]["target"])
            self.assertEqual(
                "docker.m.daocloud.io/library",
                service["build"]["args"]["SPACECHAT_BASE_IMAGE_PREFIX"],
            )
        self.assertEqual(
            "service_completed_successfully",
            services["server"]["depends_on"]["artifacts"]["condition"],
        )
        self.assertEqual(
            "service_healthy", services["cleanup"]["depends_on"]["server"]["condition"]
        )
        self.assertEqual("no", services["artifacts"]["restart"])
        self.assertEqual("unless-stopped", services["server"]["restart"])
        self.assertEqual("unless-stopped", services["cleanup"]["restart"])
        self.assertEqual("15s", services["server"]["stop_grace_period"])
        self.assertEqual(
            ["CMD", "/usr/local/bin/healthcheck.sh"],
            services["server"]["healthcheck"]["test"],
        )
        self.assertEqual(18081, services["server"]["ports"][0]["target"])
        self.assertEqual("18081", services["server"]["ports"][0]["published"])

    def test_defaults_allow_public_clients_and_use_download_mirrors(self):
        services = self.configuration()["services"]
        self.assertEqual(
            "0.0.0.0/0,::/0", services["server"]["environment"]["SPACECHAT_ALLOW_CIDR"]
        )
        self.assertEqual("", services["server"]["environment"]["SPACECHAT_PUBLIC_URL"])
        for name in ("artifacts", "server"):
            self.assertEqual(
                "https://goproxy.cn,direct", services[name]["build"]["args"]["GOPROXY"]
            )
        self.assertEqual(
            "https://mirrors.aliyun.com", services["artifacts"]["build"]["args"]["DEBIAN_MIRROR"]
        )
        self.assertEqual(
            "https://files.m.daocloud.io/github.com/microsoft/terminal/releases/download/"
            "v1.24.11911.0/Microsoft.WindowsTerminal_1.24.11911.0_x64.zip",
            services["artifacts"]["build"]["args"]["WINDOWS_TERMINAL_URL"],
        )

    def test_example_environment_preserves_deployment_defaults(self):
        self.assertEqual(
            self.configuration(), self.configuration(env_file=ROOT / ".env.example")
        )

    def test_private_inputs_and_writable_mounts_are_isolated(self):
        services = self.configuration()["services"]
        artifacts = self.mounts(services["artifacts"])
        server = self.mounts(services["server"])
        cleanup = self.mounts(services["cleanup"])
        self.assertEqual(
            {"/artifacts", "/run/secrets/update-signing.key", "/run/config/client-ca.pem"},
            set(artifacts),
        )
        self.assertEqual({"/artifacts", "/data", "/run/spacechat", "/tls"}, set(server))
        self.assertEqual({"/run/spacechat"}, set(cleanup))
        for mount in (
            artifacts["/run/secrets/update-signing.key"],
            artifacts["/run/config/client-ca.pem"], server["/tls"],
        ):
            self.assertEqual("bind", mount["type"])
            self.assertTrue(mount["read_only"])
            self.assertFalse(mount.get("bind", {}).get("create_host_path", False))
            self.assertTrue(pathlib.Path(mount["source"]).exists())
        self.assertEqual("release-assets", artifacts["/artifacts"]["source"])
        self.assertFalse(artifacts["/artifacts"].get("read_only", False))
        self.assertEqual("release-assets", server["/artifacts"]["source"])
        self.assertTrue(server["/artifacts"]["read_only"])
        self.assertEqual("data", server["/data"]["source"])
        self.assertFalse(server["/data"].get("read_only", False))
        self.assertEqual("admin-runtime", server["/run/spacechat"]["source"])
        self.assertEqual("admin-runtime", cleanup["/run/spacechat"]["source"])
        self.assertEqual("none", services["artifacts"]["network_mode"])
        self.assertEqual(
            ["--schedule-daily", "--socket", "/run/spacechat/admin.sock"],
            services["cleanup"]["command"],
        )

    def test_environment_overrides_reach_only_their_consumers(self):
        with tempfile.TemporaryDirectory(prefix="spacechat compose ") as directory:
            root = pathlib.Path(directory)
            key, ca, tls = root / "release.key", root / "ca.pem", root / "tls"
            key.touch()
            ca.touch()
            tls.mkdir()
            config = self.configuration(
                SPACECHAT_PORT="28081", SPACECHAT_VERSION="1.2.3",
                SPACECHAT_MINIMUM_VERSION="1.0.0",
                SPACECHAT_PUBLIC_URL="wss://chat.example/ws",
                SPACECHAT_ALLOW_CIDR="10.1.0.0/16",
                SPACECHAT_SIGNING_KEY_FILE=str(key),
                SPACECHAT_CLIENT_CA_FILE=str(ca), SPACECHAT_TLS_DIR=str(tls),
                GOPROXY="https://proxy.example",
                DEBIAN_MIRROR="https://debian.example",
                WINDOWS_TERMINAL_URL="https://downloads.example/terminal.zip",
                SPACECHAT_BASE_IMAGE_PREFIX="public.ecr.aws/docker/library",
            )
        services = config["services"]
        for service in services.values():
            self.assertEqual(
                "public.ecr.aws/docker/library",
                service["build"]["args"]["SPACECHAT_BASE_IMAGE_PREFIX"],
            )
            self.assertNotIn("SPACECHAT_BASE_IMAGE_PREFIX", service.get("environment", {}))
        artifacts, server = services["artifacts"], services["server"]
        for service in (artifacts, server):
            self.assertEqual("https://proxy.example", service["build"]["args"]["GOPROXY"])
        self.assertEqual(
            "https://downloads.example/terminal.zip",
            artifacts["build"]["args"]["WINDOWS_TERMINAL_URL"],
        )
        self.assertNotIn("WINDOWS_TERMINAL_URL", server["build"]["args"])
        self.assertEqual("https://debian.example", artifacts["build"]["args"]["DEBIAN_MIRROR"])
        self.assertNotIn("DEBIAN_MIRROR", server["build"]["args"])
        self.assertEqual("1.2.3", artifacts["environment"]["SPACECHAT_VERSION"])
        self.assertEqual("1.0.0", artifacts["environment"]["SPACECHAT_MINIMUM_VERSION"])
        self.assertEqual("wss://chat.example/ws", server["environment"]["SPACECHAT_PUBLIC_URL"])
        self.assertEqual("10.1.0.0/16", server["environment"]["SPACECHAT_ALLOW_CIDR"])
        self.assertEqual("28081", server["ports"][0]["published"])
        self.assertEqual(key, pathlib.Path(self.mounts(artifacts)["/run/secrets/update-signing.key"]["source"]))
        self.assertEqual(ca, pathlib.Path(self.mounts(artifacts)["/run/config/client-ca.pem"]["source"]))
        self.assertEqual(tls, pathlib.Path(self.mounts(server)["/tls"]["source"]))
        self.assertNotIn("SPACECHAT_SIGNING_KEY_FILE", server["environment"])


if __name__ == "__main__":
    unittest.main()
