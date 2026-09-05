"""Check held → roaming → held focus through the full simulator chat path.

Retain every reply, failed selection and authoritative score for motion-atlas.
This is an explicit capability check, not an Autopilot script or user preset.
"""

import argparse
import json
import time
from pathlib import Path
from llm_eval_client import App

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--base-url', required=True)
parser.add_argument('--output', required=True)
args = parser.parse_args()
app = App(args.base_url)
app.claim()
app.check_model()
rows = []


def score():
    return app.request('GET', '/api/motion/state').get('engine', {}).get('target', {}).get('flow', {})


def same_except_focus(before, after):
    if not before or not after:
        return False
    left, right = dict(before), dict(after)
    left.pop('gesture', None)
    right.pop('gesture', None)
    a, b = dict(before['gesture']), dict(after['gesture'])
    for key in ['focus_percent', 'focus_roam_percent']:
        a.pop(key, None)
        b.pop(key, None)
    return left == right and a == b


cases = [
    ('Use an outer range of 0 to 100. Start local strokes 25 percentage points wide held at the upper end, at 30% speed.',
     lambda a, b: b.get('min_percent') == 0 and b.get('max_percent') == 100 and b.get('gesture', {}).get('focus_width_percent') == 25 and b.get('gesture', {}).get('focus_percent') == 100 and b.get('gesture', {}).get('focus_roam_percent', 0) == 0 and b.get('gesture', {}).get('focus_mix_percent') == 100),
    ('Let the working location roam freely inside the existing range. Keep the current width, pace and other motion controls.',
     lambda a, b: b.get('gesture', {}).get('focus_roam_percent') == 100 and b.get('max_percent', 0)-b.get('min_percent', 0) > b.get('gesture', {}).get('focus_width_percent', 100) and same_except_focus(a, b)),
    ('Hold the working location at the lower end. Keep the current width, pace and other controls.',
     lambda a, b: b.get('gesture', {}).get('focus_percent') == 0 and b.get('gesture', {}).get('focus_roam_percent', 0) == 0 and same_except_focus(a, b)),
    ('Release that fixed location again and let it roam freely. Preserve every other control.',
     lambda a, b: b.get('gesture', {}).get('focus_roam_percent') == 100 and b.get('max_percent', 0)-b.get('min_percent', 0) > b.get('gesture', {}).get('focus_width_percent', 100) and same_except_focus(a, b)),
]

try:
    app.stop()
    app.request('POST', '/api/chat/sessions', {'discard_current_unsaved': True})
    settings = app.settings()
    settings['llm'].update(motion_generation_mode='creative_v2', chat_voice='utility')
    app.save(settings)
    for message, check in cases:
        before, started = score(), time.monotonic()
        raw = app.request('POST', '/api/chat/stream', {'message': message}, raw=True)
        events, kind = [], ''
        for line in raw.splitlines():
            if line.startswith('event:'):
                kind = line[6:].strip()
            if line.startswith('data:'):
                events.append({'event': kind, 'data': json.loads(line[5:])})
        after = score()
        result = next((e['data'] for e in reversed(events) if e['event'] == 'message'), {})
        valid = bool(result.get('reply')) and not any(result.get(k) for k in ['initial_malformed', 'repaired', 'semantic_fallback'])
        row = {'method': 'creative_v2', 'model': settings['llm']['model'], 'limits': settings['motion'], 'message': message, 'expected_recipe': message,
               'reply': result.get('reply', ''), 'raw': ''.join(e['data'].get('text', '') for e in events if e['event'] == 'delta'),
               'error': '; '.join(e['data'].get('message', '') for e in events if e['event'] == 'error'),
               'valid': valid, 'intent_pass': valid and check(before, after), 'before': before, 'after': after,
               'events': events, 'elapsed_ms': round(1000 * (time.monotonic() - started))}
        rows.append(row)
        Path(args.output).write_text(json.dumps({'turns': rows}, indent=2), encoding='utf-8')
        print(f'focus: valid={valid} intent={row["intent_pass"]}', flush=True)
finally:
    app.stop()
    app.close()
