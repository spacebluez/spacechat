import pathlib
import unittest


class RoomsDeploymentTests(unittest.TestCase):
    def test_installer_requires_tls_before_service_changes(self):
        root = pathlib.Path(__file__).parent
        installer = (root / "rooms" / "install.sh").read_text(encoding="utf-8")
        self.assertLess(installer.index("TLS certificate and private key are required"), installer.index("systemctl restart"))
        self.assertIn("XCHAT_TLS_CERT=/etc/xchat-rooms/tls.crt", installer)
        self.assertIn("XCHAT_TLS_KEY=/etc/xchat-rooms/tls.key", installer)
        self.assertNotIn("allow-insecure", installer)

    def test_isolated_installer_and_timer(self):
        root = pathlib.Path(__file__).parent
        installer = (root / "rooms" / "install.sh").read_text(encoding="utf-8")
        for old in ("/opt/xchat/", "/etc/xchat/", "/var/lib/xchat/", "restart xchat.service", "stop xchat"):
            self.assertNotIn(old, installer)
        for expected in ("/opt/xchat-rooms", "/etc/xchat-rooms", "/var/lib/xchat-rooms", "xchat-rooms.service"):
            self.assertIn(expected, installer)
        timer = (root / "rooms" / "xchat-rooms-cleanup.timer").read_text(encoding="utf-8")
        self.assertIn("OnCalendar=*-*-* 00:00:00 Asia/Shanghai", timer)
        self.assertIn("Unit=xchat-rooms-cleanup.service", timer)
        unit = (root / "rooms" / "xchat-rooms-cleanup.service").read_text(encoding="utf-8")
        self.assertIn("clear_history.py --yes --socket /run/xchat-rooms/admin.sock", unit)

    def test_script_defaults_to_new_instance(self):
        source = (pathlib.Path(__file__).parent / "clear_history.py").read_text(encoding="utf-8")
        self.assertIn('"/run/xchat-rooms/admin.sock"', source)
        self.assertNotIn('"/run/xchat/admin.sock"', source)

    def test_kaomoji_configuration_is_preserved_on_upgrade(self):
        root = pathlib.Path(__file__).parent
        installer = (root / "rooms" / "install.sh").read_text(encoding="utf-8")
        self.assertIn('if [ ! -e /etc/xchat-rooms/kaomoji.json ]; then', installer)
        self.assertIn('if not any(line.startswith("XCHAT_KAOMOJI=") for line in lines):', installer)
        self.assertIn("XCHAT_KAOMOJI=/etc/xchat-rooms/kaomoji.json", installer)
        build = (root.parent / "scripts" / "build.ps1").read_text(encoding="utf-8")
        self.assertIn("dist/rooms-server/kaomoji.json", build)


if __name__ == "__main__":
    unittest.main()
