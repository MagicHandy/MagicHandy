from contextlib import closing
import importlib.util
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest


def script(name):
    path = Path(__file__).resolve().parents[1] / (name + ".py")
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


replay = script("evaluate-chat-history")
scorer = script("score-chat-history")


class HistoryReplayTests(unittest.TestCase):
    def test_only_motion_and_selected_model_names_cross_the_database_boundary(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "source.db"
            source = {
                "motion": {"speed_min_percent": 18, "speed_max_percent": 66, "stroke_min_percent": 19,
                           "reverse_direction": True, "handy_model": "handy_original", "unknown": "private"},
                "device": {"playback_devices": ["handy", "second"], "handy_connection_key": "private",
                           "handy_connections": [{"id": "second", "handy_model": "handy_2_pro", "name": "private"},
                                                 {"id": "inactive", "handy_model": "handy_2_standard"},
                                                 {"handy_model": "handy_2_standard"}]},
            }
            with closing(sqlite3.connect(path)) as db, db:
                db.executescript("""
                    CREATE TABLE settings(document);
                    CREATE TABLE chat_sessions(id, created_at);
                    CREATE TABLE messages(session_id, seq, role, content, diagnostics_json, committed);
                    INSERT INTO chat_sessions VALUES(7, 0);
                    INSERT INTO messages VALUES(7, 1, 'user', 'First synthetic request', NULL, 1);
                    INSERT INTO messages VALUES(7, 2, 'assistant', 'A', '{"motion_action":"start"}', 1);
                    INSERT INTO messages VALUES(7, 3, 'user', 'Second synthetic request', NULL, 1);
                    INSERT INTO messages VALUES(7, 4, 'assistant', 'B', '{"motion_action":"start"}', 1);
                """)
                db.execute("INSERT INTO settings VALUES(?)", (json.dumps(source),))
            data = replay.read_conversations(path, set())
            self.assertEqual(data["handy_models"], ["handy_original", "handy_2_pro"])
            self.assertEqual(data["motion"], {k: v for k, v in source["motion"].items() if k != "unknown"})
            self.assertNotIn("private", json.dumps(data))
            self.assertNotIn("device", data)
            self.assertTrue(data["sessions"][0]["events"][1]["reset"])
            literal = replay.read_conversations(path, set(), infer_stops=False)
            self.assertFalse(literal["sessions"][0]["events"][1]["reset"])
            self.assertFalse(literal["infer_stops"])
            self.assertEqual(data["source_id"], literal["source_id"])
            source["device"]["handy_connection_key"] = "replacement"
            with closing(sqlite3.connect(path)) as db, db:
                db.execute("UPDATE settings SET document=?", (json.dumps(source),))
            self.assertEqual(data["source_id"], replay.read_conversations(path, set())["source_id"])
            with closing(sqlite3.connect(path)) as db, db:
                db.execute("UPDATE chat_sessions SET id=8 WHERE id=7")
                db.execute("UPDATE messages SET session_id=8 WHERE session_id=7")
            self.assertNotEqual(data["source_id"], replay.read_conversations(path, set())["source_id"])

    def test_direction_geometry_saturation_and_paired_cases(self):
        header = {"kind": "profile", "schema": "chat_history_replay.v2",
                  "settings": {"speed_min_percent": 20, "speed_max_percent": 80}}
        a = {"min_percent": 15, "max_percent": 72, "speed_percent": 38}
        b = {**a, "speed_percent": 48}
        row = {"kind": "turn", "session": 1, "step": 1, "first_pass": True, "plan_changed": True,
               "was_running": True, "running": True, "before": a, "after": b, "elapsed_ms": 30}
        high = {**a, "speed_percent": 80}
        cap = {**row, "step": 2, "before": high, "after": high, "plan_changed": False}
        recall = {**row, "step": 3, "before": high, "after": high, "plan_changed": False}
        labels = [{"session": 1, "step": 1, "kind": "up"}, {"session": 1, "step": 2, "kind": "max"},
                  {"session": 1, "step": 3, "kind": "restore"}]
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "replay.jsonl"
            path.write_text("\n".join(map(json.dumps, [header, row, cap, recall])), encoding="utf-8")
            report, previous = scorer.score(path, labels, 1)
            self.assertEqual(report["scores"]["pace"]["passed"], 2)
            self.assertEqual(report["scores"]["restore"]["unscorable"], 1)
            row["after"] = {**b, "min_percent": 16}
            cap["plan_changed"] = True
            path.write_text("\n".join(map(json.dumps, [header, row, cap, recall])), encoding="utf-8")
            report, current = scorer.score(path, labels, 1)
            self.assertEqual(report["first_pass"], 3)
            self.assertEqual(report["scores"]["pace"]["passed"], 0)
            self.assertEqual(scorer.paired_changes(previous, current)["pace"], {"scored_in_both": 2, "gained": 0, "lost": 2})

    def test_comparisons_require_matching_sources_geometry_and_sampling(self):
        a = {"schema": "chat_history_replay.v2", "source_id": "source", "labels_bound": True, "model": "gemma-e4b",
             "provider": "llama_cpp", "capability": "simple", "context_size": 16384,
             "gguf_sha256": "weights", "template_fix": "gemma4", "max_output_tokens": 256,
             "settings": {"stroke_min_percent": 19}, "infer_stops": False, "seed_mode": "provider_default",
             "requested_sampling": [{"temperature": .6}], "binary_sha256": "baseline"}
        self.assertEqual(scorer.compare_profiles(a, {**a, "binary_sha256": "candidate"}, False), [])
        for key, changed in [("settings", {}), ("infer_stops", True), ("context_size", 32768),
                             ("requested_sampling", [{"temperature": .8}]), ("gguf_sha256", "other")]:
            b = {**a, key: changed}
            with self.assertRaisesRegex(ValueError, key):
                scorer.compare_profiles(a, b, False)
            self.assertEqual(scorer.compare_profiles(a, b, True), [key])
        for allow in (True, False):
            with self.assertRaisesRegex(ValueError, "identities"):
                scorer.compare_profiles(a, {**a, "source_id": "different"}, allow)
        old = {"schema": "chat_history_replay.v1"}
        with self.assertRaisesRegex(ValueError, "unverified"):
            scorer.compare_profiles(old, old, False)
        self.assertIn("source_id", scorer.compare_profiles(old, old, True))
        with self.assertRaisesRegex(ValueError, "source_id"):
            scorer.compare_profiles({**a, "source_id": ""}, {**a, "source_id": ""}, False)
        unbound = {**a, "labels_bound": False}
        with self.assertRaisesRegex(ValueError, "labels_bound"):
            scorer.compare_profiles(unbound, unbound, False)
        self.assertEqual(scorer.compare_profiles(unbound, unbound, True), ["labels_bound"])
        deterministic = {**a, "requested_sampling": []}
        self.assertEqual(scorer.compare_profiles(deterministic, deterministic, False), [])

    def test_exact_values_limits_and_annotation_identity(self):
        header = {"kind": "profile", "schema": "chat_history_replay.v2", "source_id": "source",
                  "settings": {"speed_min_percent": 20, "speed_max_percent": 80}}
        row = {"kind": "turn", "session": 1, "first_pass": True, "outcome": "first_pass",
               "was_running": True, "running": True, "plan_changed": True, "elapsed_ms": 15}
        def flow(speed):
            return {"min_percent": 12, "max_percent": 77, "speed_percent": speed}
        samples = [{**row, "step": i, "before": flow(x), "after": flow(y)}
                   for i, (x, y) in enumerate([(40, 55), (55, 80), (80, 20)], 1)]
        labels = [{"session": 1, "step": i, "kind": kind, "value": value}
                  for i, (kind, value) in enumerate([("set", 55), ("adjust", 40), ("adjust", -90)], 1)]
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "capture.jsonl"
            path.write_text("\n".join(map(json.dumps, [header, *samples])), encoding="utf-8")
            report, _ = scorer.score(path, {"source_id": "source", "labels": labels}, 32)
            self.assertEqual((report["scores"]["pace"]["passed"], report["completed"]), (3, 3))
            self.assertEqual(report["outcomes"], {"first_pass": 3})
            self.assertTrue(report["profile"]["labels_bound"])
            self.assertFalse(scorer.score(path, labels, 32)[0]["profile"]["labels_bound"])
            with self.assertRaisesRegex(ValueError, "source snapshot"):
                scorer.score(path, {"source_id": "different", "labels": labels}, 32)
            with self.assertRaisesRegex(ValueError, "saved speed limits"):
                scorer.score(path, [{**labels[0], "value": 81}], 32)
            with self.assertRaisesRegex(ValueError, "100 points"):
                scorer.score(path, [{**labels[1], "value": 101}], 32)

    def test_partial_sampler_metadata_and_zero_call_stops(self):
        header = {"kind": "profile", "schema": "chat_history_replay.v2", "settings": {}}
        row = {"kind": "turn", "session": 1, "first_pass": True, "plan_changed": False,
               "running": False, "calls": 0, "step": 1}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "capture.jsonl"
            def sampling(*rows):
                path.write_text("\n".join(map(json.dumps, [header, *rows])), encoding="utf-8")
                return scorer.score(path, [], 32)[0]["profile"].get("requested_sampling")
            self.assertEqual(sampling(row), [])
            generated = {**row, "step": 2, "calls": 1, "requested_sampling": {"temperature": .3}}
            self.assertEqual(sampling(row, generated), [{"temperature": .3}])
            self.assertIsNone(sampling(generated, {**row, "calls": 1}))


if __name__ == "__main__":
    unittest.main()
