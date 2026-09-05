"""Exercise live continuous-mode switches without clearing the previous score.

Requires an isolated simulator app with a working LLM. Retains accepted targets,
failed decisions and dispatch traces for the shared-engine visual review.
"""

import argparse
import json
import time
from pathlib import Path
from llm_eval_client import App

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--base-url', required=True)
parser.add_argument('--output', required=True)
parser.add_argument('--seconds', type=int, default=25)
args = parser.parse_args()
app = App(args.base_url)
app.claim()
app.check_model()
report = {'sessions': []}


def save():
    Path(args.output).write_text(json.dumps(report, indent=2), encoding='utf-8')


try:
    app.stop()
    app.request('POST', '/api/chat/sessions', {'discard_current_unsaved': True})
    settings = app.settings()
    settings['llm']['chat_voice'] = 'utility'
    settings['autopilot'].update(motion_change_level=8, speech_cadence='custom',
                                speech_min_seconds=8, speech_max_seconds=12,
                                speech_motion_authority='chat_only')
    app.save(settings)
    for mode in ['layered', 'creative_v2', 'layered', 'creative_v2']:
        before = app.request('GET', '/api/motion/state').get('engine', {}).get('target')
        app.request('PUT', '/api/settings/llm-motion-mode', {'mode': mode})
        # Keep the old engine score: clearing it here would hide the regression.
        retained = app.request('GET', '/api/motion/state').get('engine', {}).get('target')
        assert retained == before, 'Mode selection unexpectedly replaced the active score'
        run = {'mode': mode, 'model': settings['llm']['model'], 'settings': settings['motion'],
               'previous_target': before, 'targets': [], 'messages': [], 'events': []}
        report['sessions'].append(run)
        app.request('POST', '/api/modes/start', {'mode': 'autopilot'})
        started, last_plan, last_event = time.monotonic(), '', ''
        while time.monotonic() - started < args.seconds:
            time.sleep(.3)
            seconds = round(time.monotonic() - started, 3)
            engine = app.request('GET', '/api/motion/state').get('engine', {})
            if engine.get('plan_id') and engine['plan_id'] != last_plan:
                last_plan = engine['plan_id']
                run['targets'].append({'at': seconds, 'target': engine['target'], 'plan_id': last_plan})
            status = app.request('GET', '/api/modes')
            serialized = json.dumps(status, sort_keys=True)
            if serialized != last_event:
                run['events'].append({'at': seconds, 'state': status})
                last_event = serialized
        run['seconds'] = round(time.monotonic() - started, 3)
        run['trace'] = app.request('GET', '/api/traces')
        run['messages'] = app.request('GET', '/api/chat/messages').get('messages', [])
        active = app.request('GET', '/api/motion/state').get('engine', {}).get('target', {})
        run['passed'] = bool(active.get('flow')) and bool(active['flow'].get('gesture')) == (mode == 'creative_v2')
        run['passed'] = run['passed'] and any(event['state'].get('decision_source') == 'model' for event in run['events'])
        save()
        print(f'{mode}: {len(run["targets"])} observed targets, switched={run["passed"]}', flush=True)
        assert run['passed'], 'The restarted Autopilot did not accept the selected score grammar'
finally:
    try:
        app.stop()
        report['stopped'] = not app.request('GET', '/api/motion/state').get('engine', {}).get('running', False)
        report['trace'] = app.request('GET', '/api/traces')
    finally:
        app.close()
        save()
