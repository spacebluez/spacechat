import contextlib
import datetime
import http.server
import io
import json
import os
import socketserver
import tempfile
import threading
import unittest
from unittest import mock

import clear_history


@contextlib.contextmanager
def fake_admin(status=200, deleted=3):
    class Handler(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            self.server.paths.append(self.path)
            body = json.dumps({"deleted": deleted}).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass

    with tempfile.TemporaryDirectory() as directory:
        path = os.path.join(directory, "admin.sock")
        with socketserver.UnixStreamServer(path, Handler) as server:
            server.paths = []
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                yield path, server
            finally:
                server.shutdown()
                thread.join()


class ClearHistoryTests(unittest.TestCase):
    def test_next_midnight_uses_fixed_shanghai_time(self):
        now = datetime.datetime(2026, 9, 22, 15, 30, tzinfo=datetime.timezone.utc)
        self.assertEqual(1800, clear_history.seconds_until_next_midnight(now))
        midnight = datetime.datetime(2026, 9, 22, 16, 0, tzinfo=datetime.timezone.utc)
        self.assertEqual(24 * 60 * 60, clear_history.seconds_until_next_midnight(midnight))

    def test_schedule_failure_waits_for_the_next_midnight(self):
        sleeps = []

        def stop_after_second_sleep(seconds):
            sleeps.append(seconds)
            if len(sleeps) == 2:
                raise StopIteration

        clear = mock.Mock(side_effect=OSError("offline"))
        with self.assertRaises(StopIteration):
            clear_history.run_daily(
                "/run/admin.sock",
                now=lambda zone: datetime.datetime(2026, 9, 22, 15, 30, tzinfo=zone),
                sleep=stop_after_second_sleep,
                clear=clear,
                stderr=io.StringIO(),
            )
        self.assertEqual([1800, 1800], sleeps)
        self.assertEqual(1, clear.call_count)

    def test_fractional_seconds_do_not_run_early_and_retry_at_midnight(self):
        clock = datetime.datetime(2026, 9, 22, 15, 30, 0, 500000,
                                  tzinfo=datetime.timezone.utc)
        sleeps = []

        def advance_until_second_sleep(seconds):
            nonlocal clock
            sleeps.append(seconds)
            if len(sleeps) == 2:
                raise StopIteration
            clock += datetime.timedelta(seconds=seconds)

        clear = mock.Mock(side_effect=OSError("offline"))
        with self.assertRaises(StopIteration):
            clear_history.run_daily(
                "/run/admin.sock", now=lambda zone: clock.astimezone(zone),
                sleep=advance_until_second_sleep, clear=clear, stderr=io.StringIO(),
            )
        self.assertEqual([1800, 86400], sleeps)
        self.assertEqual(datetime.date(2026, 9, 23), clock.astimezone(clear_history.SHANGHAI).date())
        self.assertEqual(1, clear.call_count)

    def test_schedule_and_yes_modes_are_mutually_exclusive(self):
        with contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit) as error:
                clear_history.main(["--schedule-daily", "--yes"])
        self.assertEqual(2, error.exception.code)

    def test_clear_uses_private_post_endpoint(self):
        with fake_admin() as (path, server):
            self.assertEqual(clear_history.clear_history(path), 3)
            self.assertEqual(server.paths, ["/clear-history"])

    def test_empty_room_is_success(self):
        with fake_admin(deleted=0) as (path, server):
            self.assertEqual(clear_history.clear_history(path), 0)

    def test_failure_does_not_report_success_or_retry(self):
        with fake_admin(status=500) as (path, server):
            with contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(clear_history.main(["--yes", "--socket", path]), 1)
            self.assertEqual(server.paths, ["/clear-history"])

    def test_cancel_does_not_connect(self):
        with mock.patch("builtins.input", return_value="no"):
            with mock.patch.object(clear_history, "clear_history") as request:
                with contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(clear_history.main([]), 2)
                request.assert_not_called()

    def test_confirmation_executes_once(self):
        with mock.patch("builtins.input", return_value="CLEAR"):
            with mock.patch.object(clear_history, "clear_history", return_value=7) as request:
                with contextlib.redirect_stdout(io.StringIO()) as output:
                    self.assertEqual(clear_history.main([]), 0)
                request.assert_called_once()
                self.assertIn("7", output.getvalue())

    def test_missing_socket_has_nonzero_exit(self):
        with tempfile.TemporaryDirectory() as directory:
            with contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(clear_history.main(["--yes", "--socket", os.path.join(directory, "missing.sock")]), 1)


if __name__ == "__main__":
    unittest.main()
