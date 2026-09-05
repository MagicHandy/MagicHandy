"""Evaluate a coherent 24-turn conversation through production SSE chat.

Creates a fresh disposable session and keeps motion stopped. Read the replies
as well as lexical recall checks: a correct noun can coexist with a wrong fact.
"""

import argparse, json, time
from pathlib import Path
from llm_eval_client import App

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("--output", required=True)
p.add_argument("--base-url", required=True)
p.add_argument("--mode", default="creative_v2")
p.add_argument("--voice", default="warm")
args = p.parse_args()
a = App(args.base_url)
a.claim()
a.check_model()
rows = []
turns = [
    (
        "Call me Rowan. Let us have a quiet conversation about our imaginary visit to an old hilltop observatory. My blue notebook is important to me. We planned to watch the Perseid meteor shower, then share cinnamon tea. Leave the device stopped throughout this conversation.",
        None,
    ),
    (
        "I meant a green notebook, not blue. Please use the corrected color from now on. I prefer you to talk with me rather than narrate my actions.",
        None,
    ),
    ("The wind is getting stronger on the walk uphill. What do you notice?", None),
    (
        "The caretaker has left us a brass key under the bench. We pick it up and enter the observatory.",
        None,
    ),
    ("There is a handwritten note near the telescope. What might it say?", None),
    (
        "I like the idea that people before us watched this same sky. Stay with that thought.",
        None,
    ),
    (
        "Let us move from the telescope to the sheltered balcony. I set the key on its small wooden table.",
        None,
    ),
    ("What do you think makes a shared silence comfortable?", None),
    ("I would enjoy a small joke about astronomers right now.", None),
    (
        "That helps. Bring us gently back to where we are, without restarting our visit.",
        None,
    ),
    (
        "Before we continue, what is my name, what color is my notebook, and which meteor shower did we come to watch?",
        ["rowan", "green", "perseid"],
    ),
    (
        "Clouds have covered the sky, so we decide to postpone meteor watching until tomorrow. Tonight we will read by the balcony lamp instead.",
        None,
    ),
    (
        "I would like a thoughtful question about curiosity, connected to our evening.",
        None,
    ),
    (
        "For me curiosity is mostly about noticing something familiar differently. Respond to that thought.",
        None,
    ),
    (
        "There is a soft rain against the roof now. Keep the scene grounded in what has happened.",
        None,
    ),
    ("What kind of short book would suit this moment? Give me one idea.", None),
    ("A travel diary sounds right. Imagine a single line from its opening page.", None),
    ("The lamp flickers once and steadies. We stay seated and keep reading.", None),
    (
        "I am enjoying the slower conversation. You do not need to announce a new plan each time.",
        None,
    ),
    (
        "What had we originally planned to do tonight, and why did we change our plan?",
        ["meteor", "cloud"],
    ),
    ("And where did I last put the brass key?", ["table"]),
    ("What drink were we going to share after watching the sky?", ["cinnamon"]),
    (
        "Continue our current evening in two sentences. Keep our earlier choices consistent.",
        None,
    ),
    (
        "What is one detail from early in our conversation that could matter when we leave?",
        None,
    ),
]


def chat(message):
    raw = a.request("POST", "/api/chat/stream", {"message": message}, raw=True)
    events = []
    for block in raw.replace("\r\n", "\n").split("\n\n"):
        event = "message"
        data = []
        for line in block.splitlines():
            if line.startswith("event:"):
                event = line[6:].strip()
            if line.startswith("data:"):
                data.append(line[5:].strip())
        if data:
            try:
                events.append({"event": event, "data": json.loads("\n".join(data))})
            except ValueError:
                events.append({"event": event, "data": "\n".join(data)})
    return events


try:
    assert a.request("GET", "/api/state").get("motion_simulated")
    a.stop()
    a.request("POST", "/api/chat/sessions", {"discard_current_unsaved": True})
    s = a.settings()
    s["llm"]["motion_generation_mode"] = args.mode
    s["llm"]["chat_voice"] = args.voice
    a.save(s)
    for index, (message, expected) in enumerate(turns):
        start = time.monotonic()
        events = chat(message)
        reply = next(
            (e["data"]["reply"] for e in reversed(events) if e["event"] == "message"),
            "",
        )
        stopped = (
            not a.request("GET", "/api/motion/state").get("engine", {}).get("running")
        )
        row = {
            "turn": index + 1,
            "input": message,
            "reply": reply,
            "events": events,
            "seconds": round(time.monotonic() - start, 2),
            "stopped": stopped,
            "expected": expected,
            "recall_pass": (
                None if expected is None else all(v in reply.lower() for v in expected)
            ),
        }
        rows.append(row)
        Path(args.output).write_text(
            json.dumps(
                {"mode": args.mode, "model": s["llm"]["model"], "turns": rows}, indent=2
            ),
            encoding="utf-8",
        )
        print(
            f'Turn {index+1}: recall={row["recall_pass"]}, stopped={stopped}, {row["seconds"]}s',
            flush=True,
        )
finally:
    a.stop()
    a.close()
