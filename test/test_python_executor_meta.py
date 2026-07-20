import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class PythonExecutorMetaTest(unittest.TestCase):
    def test_python312_through_python314_system_executors(self):
        for minor in ("3.12", "3.13", "3.14"):
            path = ROOT / "build" / "executor-meta" / f"python{minor}_meta.json"
            meta = json.loads(path.read_text(encoding="utf-8"))
            function = meta["funcMetaData"]
            self.assertEqual(function["name"], f"0-system-faasExecutorPython{minor}")
            self.assertEqual(function["runtime"], f"python{minor}")
            self.assertIn(f"faasExecutorPython{minor}", function["functionUrn"])
            self.assertIn(f"faasExecutorPython{minor}", function["functionVersionUrn"])
            self.assertEqual(
                meta["codeMetaData"]["code_path"],
                "/home/sn/system-function-packages/executor-function/python3.11",
            )
            self.assertEqual(
                set(function["hookHandler"]),
                {"call", "checkpoint", "init", "recover", "shutdown", "signal"},
            )


if __name__ == "__main__":
    unittest.main()
