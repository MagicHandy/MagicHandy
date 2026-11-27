import os
import sys
import tempfile
import unittest
from contextlib import ExitStack
from email.message import Message
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from packaging.markers import default_environment

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import hub_xet


class HubXetTest(unittest.TestCase):
    def setUp(self):
        env = patch.dict(os.environ, {"HF_HUB_DISABLE_XET": "0"})
        env.start()
        self.addCleanup(env.stop)
        self.probe = self.enter_patch(patch.object(hub_xet, "xet_usable"))
        self.install = self.enter_patch(patch.object(hub_xet.subprocess, "run"))
        self.hub = self.enter_patch(patch.object(hub_xet.metadata, "distribution"))
        self.metadata = Message()
        self.metadata["Provides-Extra"] = "hf-xet"
        self.hub.return_value = SimpleNamespace(version="0.36.2", metadata=self.metadata)
        tmp = tempfile.TemporaryDirectory(prefix="managed voice ")
        self.addCleanup(tmp.cleanup)
        self.candidate = Path(tmp.name)
        self.python = Path(".venv", "Scripts", "python.exe")
        self.enter_patch(patch.object(sys, "prefix", str(self.candidate / ".venv")))
        self.enter_patch(patch.object(sys, "executable", str(self.candidate / self.python)))

    def enter_patch(self, patcher):
        self.addCleanup(patcher.stop)
        return patcher.start()

    def test_opt_out_skips_probe_and_install(self):
        for value in ("1", "On", "YES", "true"):
            with self.subTest(value=value), patch.dict(os.environ, {"HF_HUB_DISABLE_XET": value}):
                self.assertIn("disabled", hub_xet.select_backend("uv"))
        self.probe.assert_not_called()
        self.install.assert_not_called()

    def test_ready_environment_and_check_only_never_install(self):
        self.probe.side_effect = [True, False]
        self.assertEqual(hub_xet.select_backend("uv"), "Xet")
        self.assertEqual(hub_xet.main(), 0)
        self.install.assert_not_called()

    def test_install_uses_resolved_hub_and_rechecks_native_wheel(self):
        self.probe.side_effect = [False, True]
        self.install.return_value.returncode = 0
        self.assertEqual(hub_xet.main("uv"), 0)
        args = self.install.call_args.args[0]
        self.assertIn("huggingface-hub[hf_xet]==0.36.2", args)
        self.assertEqual(args[args.index("--python") + 1], str(self.python))
        self.assertEqual(args[args.index("--only-binary") + 1], "hf-xet")
        self.assertEqual(args[args.index("--reinstall-package") + 1], "hf-xet")
        self.assertEqual(self.install.call_args.kwargs["cwd"], self.candidate)
        self.assertEqual(self.probe.call_count, 2)

    def test_failed_install_and_broken_native_wheel_select_http(self):
        self.probe.return_value = False
        for code in (1, 0):
            with self.subTest(uv_exit=code):
                self.install.return_value.returncode = code
                self.assertEqual(hub_xet.main("uv"), 1)
        self.install.side_effect = OSError("unavailable")
        self.assertEqual(hub_xet.main("uv"), 1)

    def test_system_python_and_hub_without_extra_are_left_alone(self):
        self.probe.return_value = False
        with patch.object(sys, "prefix", sys.base_prefix):
            self.assertIn("HTTP", hub_xet.select_backend("uv"))
        del self.metadata["Provides-Extra"]
        self.assertIn("HTTP", hub_xet.select_backend("uv"))
        self.install.assert_not_called()


class NativeProbeTest(unittest.TestCase):
    def test_windows_amd64_checks_the_hub_extra_and_native_entry_points(self):
        native = SimpleNamespace(PyXetDownloadInfo=object, download_files=lambda: None)
        modules = {
            "hf_xet": native,
            "huggingface_hub.utils._runtime": SimpleNamespace(is_xet_available=lambda: True),
        }
        windows = dict(default_environment(), platform_machine="AMD64")
        # Hub 0.36.2's automatic dependency marker uses lowercase amd64.
        # Its explicit Xet extra still supplies the bounds on Windows.
        requirements = [
            'hf-xet>=1.1.3,<2; platform_machine == "amd64"',
            'hf-xet>=1.1.2,<2; extra == "hf-xet"',
        ]
        with ExitStack() as mocks:
            mocks.enter_context(patch.dict(sys.modules, modules))
            mocks.enter_context(patch("packaging.markers.default_environment", side_effect=windows.copy))
            mocks.enter_context(patch.object(hub_xet.metadata, "requires", return_value=requirements))
            version = mocks.enter_context(patch.object(hub_xet.metadata, "version"))
            cases = [("1.1.1", False), ("1.1.2", True), ("1.8.0", True), ("2.0.0", False)]
            for installed, expected in cases:
                with self.subTest(installed=installed):
                    version.return_value = installed
                    self.assertEqual(hub_xet.probe_current_python(), expected)
            version.return_value = "1.8.0"
            native.download_files = None
            self.assertFalse(hub_xet.probe_current_python())
