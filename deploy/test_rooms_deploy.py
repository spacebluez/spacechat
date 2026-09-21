import pathlib
import subprocess
import tempfile
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
            '"$release/installers"',
            "/etc/xchat-rooms/update-public.key.new",
            "/etc/xchat-rooms/update-public.key",
            "/opt/xchat-rooms/installers.new",
            "/opt/xchat-rooms/installers.previous",
            "/opt/xchat-rooms/installers",
            "-validate-updates",
            "-update-dir",
            "-installer-dir",
            "-update-public-key-file",
            "find",
            "-type l",
        ):
            self.assertIn(expected, installer)
        self.assertIn('install -m 0644 "$release/update-public.key"', installer)
        self.assertLess(installer.index("-validate-updates"), installer.index("updates.previous"))
        for backup in (
            "/opt/xchat-rooms/clear_history.py.previous",
            "/etc/systemd/system/xchat-rooms.service.previous",
            "/etc/systemd/system/xchat-rooms-cleanup.service.previous",
            "/etc/systemd/system/xchat-rooms-cleanup.timer.previous",
        ):
            self.assertIn(backup, installer)
        self.assertIn("systemctl restart xchat-rooms-cleanup.timer || true", installer)

        service = (root / "rooms" / "xchat-rooms.service").read_text(encoding="utf-8")
        self.assertIn("-update-dir /opt/xchat-rooms/updates", service)
        self.assertIn("-installer-dir /opt/xchat-rooms/installers", service)
        self.assertIn("-update-public-key-file /etc/xchat-rooms/update-public.key", service)

    def test_atomic_swap_rolls_back_when_interrupted(self):
        root = pathlib.Path(__file__).parent
        installer = (root / "rooms" / "install.sh").read_text(encoding="utf-8")
        for expected in (
            "updates_had_current=",
            "installers_had_current=",
            "restore_directory",
            "restore_file",
            "trap 'exit 129' HUP",
            "trap 'exit 130' INT",
            "trap 'exit 143' TERM",
            "trap - EXIT HUP INT TERM",
        ):
            self.assertIn(expected, installer)
        for obsolete in ("updates_swapped=", "installers_swapped=", "key_swapped="):
            self.assertNotIn(obsolete, installer)
        self.assertLess(installer.index("trap rollback EXIT"), installer.index("mv /opt/xchat-rooms/updates /opt/xchat-rooms/updates.previous"))

    def test_restore_helpers_cover_each_interruption_point(self):
        installer = (pathlib.Path(__file__).parent / "rooms" / "install.sh").read_text(encoding="utf-8")
        helpers = installer[installer.index("restore_directory() {"):installer.index("rollback() {")]
        cases = (
            ("before-old-move", True, False, True, 1, b"old"),
            ("after-old-move", False, True, True, 1, b"old"),
            ("after-new-move", True, True, False, 1, b"old"),
            ("fresh-install-after-new-move", True, False, False, 0, None),
        )
        for name, has_current, has_previous, has_staged, had_current, expected in cases:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                root = pathlib.Path(temporary)
                if has_current:
                    (root / "current").mkdir()
                    (root / "current" / "value").write_bytes(b"new" if has_previous or not has_staged else b"old")
                if has_previous:
                    (root / "previous").mkdir()
                    (root / "previous" / "value").write_bytes(b"old")
                if has_staged:
                    (root / "staged").mkdir()
                    (root / "staged" / "value").write_bytes(b"new")
                script = helpers + '\nrestore_directory "$1/current" "$1/previous" "$1/staged" "$2"\n'
                subprocess.run(["sh", "-c", script, "restore-test", temporary, str(had_current)], check=True)
                value = root / "current" / "value"
                self.assertEqual(value.read_bytes() if value.exists() else None, expected)

    def test_script_defaults_to_new_instance(self):
        source = (pathlib.Path(__file__).parent / "clear_history.py").read_text(encoding="utf-8")
        self.assertIn('"/run/xchat-rooms/admin.sock"', source)
        self.assertNotIn('"/run/xchat/admin.sock"', source)


if __name__ == "__main__":
    unittest.main()
