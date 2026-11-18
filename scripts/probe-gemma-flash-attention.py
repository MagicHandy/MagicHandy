"""Check E4B table lookups against a running llama-server using native greedy completion."""

import argparse
import hashlib
import json
from urllib.request import Request, urlopen


def request(base_url, route, payload):
    http_request = Request(
        base_url.rstrip("/") + route,
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urlopen(http_request, timeout=150) as response:
        return json.load(response)


def table_prompt(count):
    codes = [f"{(37 * index + 317) % 997:03d}" for index in range(1, count + 1)]
    entries = [f"Bin {index:03d} = K{code}" for index, code in enumerate(codes, 1)]
    target = max(1, count // 2)
    question = (
        f"Look up bin {target:03d} in this table. Reply with its code only.\n"
        + "\n".join(entries)
        + f"\nWhat is the code for bin {target:03d}?"
    )
    # Prefill the answer up to K. The native decoder only has to copy three digits.
    prompt = (
        "<bos><|turn>user\n" + question + "<turn|>\n<|turn>model\n<|channel>thought\n<channel|>"
        + f"\nThe code for bin {target:03d} is K"
    )
    return prompt, codes[target - 1]


def probe(base_url, count):
    prompt, expected = table_prompt(count)
    tokenized = request(base_url, "/tokenize", {
        "content": prompt, "add_special": False, "parse_special": True,
    })
    tokens = tokenized["tokens"]
    result = request(base_url, "/completion", {
        "prompt": tokens, "n_predict": 3, "temperature": 0, "seed": 12345,
        "cache_prompt": False, "return_tokens": True,
    })
    output = result["content"].strip()
    input_digest = hashlib.sha256(json.dumps(tokens, separators=(",", ":")).encode()).hexdigest()
    return {
        "rows": count, "expected": expected, "actual": output, "exact": output == expected,
        "prompt_tokens": len(tokens), "input_sha256": input_digest, "generated_ids": result["tokens"],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:8080")
    parser.add_argument("--rows", nargs="+", type=int, default=[3, 19, 73, 211])
    args = parser.parse_args()
    if min(args.rows) < 1:
        parser.error("the table must have at least one row")
    passed = True
    for count in args.rows:
        result = probe(args.url, count)
        print(json.dumps(result), flush=True)
        passed = passed and result["exact"]
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
