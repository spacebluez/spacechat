import pathlib
import unittest


class RoomsDeploymentTests(unittest.TestCase):
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

    def test_signed_updates_are_validated_before_atomic_swap(self):
        root = pathlib.Path(__file__).parent
        installer = (root / "rooms" / "install.sh").read_text(encoding="utf-8")
        for expected in (
            '"$release/updates"',
            "/opt/xchat-rooms/updates.new",
            "/opt/xchat-rooms/updates.previous",
            "/opt/xchat-rooms/updates",
            '"$release/update-public.key"',
            "/etc/xchat-rooms/update-public.key.new",
            "/etc/xchat-rooms/update-public.key",
            "-validate-updates",
            "-update-dir",
            "-update-public-key-file",
            "find",
            "-type l",
        ):
            self.assertIn(expected, installer)
        self.assertIn('install -m 0644 "$release/update-public.key"', installer)
        self.assertLess(installer.index("-validate-updates"), installer.index("updates.previous"))

        service = (root / "rooms" / "xchat-rooms.service").read_text(encoding="utf-8")
        self.assertIn("-update-dir /opt/xchat-rooms/updates", service)
        self.assertIn("-update-public-key-file /etc/xchat-rooms/update-public.key", service)

    def test_script_defaults_to_new_instance(self):
        source = (pathlib.Path(__file__).parent / "clear_history.py").read_text(encoding="utf-8")
        self.assertIn('"/run/xchat-rooms/admin.sock"', source)
        self.assertNotIn('"/run/xchat/admin.sock"', source)


if __name__ == "__main__":
    unittest.main()
