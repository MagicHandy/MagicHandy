# Standard-library review harness. Use a fresh isolated app, not a user session.
# Results preserve wrong selections and raw replies for render-motion-atlas.py.
import argparse
import json
import time
import urllib.request
from pathlib import Path
parser = argparse.ArgumentParser(description="Evaluate an isolated review app's Lab conversation; takes controller ownership and runs preview-only Autopilot. Never targets physical motion.")
parser.add_argument('--base-url', required=True, help='Isolated review app URL with Labs enabled and a working provider')
parser.add_argument('--model', action='append', required=True, help='Installed model name; repeat to compare models')
parser.add_argument('--output', type=Path, default=Path('.scratch/lab-conversation-live.json'))
args = parser.parse_args()
BASE = args.base_url.rstrip('/')
HEAD={'Content-Type':'application/json','X-MagicHandy-Client-ID':'lab-conversation-evaluation'}
def request(method,path,body=None):
 data=None if body is None else json.dumps(body).encode()
 with urllib.request.urlopen(urllib.request.Request(BASE+path,data=data,method=method,headers=HEAD),timeout=150) as r:return json.load(r)
def own(): request('POST','/api/controller/takeover',{})
models=args.model
cases=[
('Return to base 0 every stroke, with reach wandering irregularly over long gradual trends. Keep pace unchanged.','Base-anchored drift'),
('Return to tip 100 every stroke, with reach wandering irregularly over long gradual trends. Keep pace unchanged.','Tip-anchored drift'),
('Keep the middle fixed and let both endpoints wander symmetrically with gradual irregular changes in width. Keep pace unchanged.','Centered drift'),
('Full-length strokes, with longer softer turnarounds at both ends. Fixed reach and even pace.','Soft turnarounds'),
('Let width vary irregularly around the middle, but keep the cycle beat even as reach changes. Short strokes should take about as long as broad strokes.','Even-beat variety'),
('Four short cycles in the lower region, then four middle, four upper, four middle, repeating. Blend each transition.','Three-zone tour'),
('Move a window through the band while its width wanders gradually between 20 and 65. Neither endpoint anchored.','Breathing window'),
('Return to base 0 while reach varies in smooth repeating waves. Keep pace independent.','Base-anchored variety'),
('A constant 40-wide window that travels from lower to upper and back. Preserve the width.','Traveling window'),
('Keep every stroke full length, but let pace rise and slow in a gradual wave.','Pace wave'),
]
turns=[]
output=args.output
output.parent.mkdir(parents=True,exist_ok=True)
def save(): output.write_text(json.dumps({'turns':turns},ensure_ascii=False,indent=2),encoding='utf-8')
for model in models:
 for method in ['library_actions','library_descriptive']:
  for message,expected in cases:
   own(); state=request('GET','/api/labs/llm'); initial=dict(state['current']); initial.update(min_percent=5,max_percent=95,speed_percent=25,range_floor_percent=25,anchor_percent=0,memory_cycles=8,pace_variation_percent=10,seed=17,steps=[],layers=[],range_ceiling_percent=0,loop_cycles=0,variation_mode='waves',turn_softness_percent=0,cadence_hold_percent=0)
   state=request('POST','/api/labs/llm/reset',{'spec':initial})
   state=request('POST','/api/labs/llm/chat',{'message':message,'method':method,'model':model,'revision':state['revision'],'schema_guided':True})
   row=state['turns'][-1];row['expected_recipe']=expected;row['intent_pass']=row.get('recipe_name')==expected and row['after']['speed_percent']==25
   turns.append(row);save();print(json.dumps({'model':model,'method':method,'expected':expected,'selected':row.get('recipe_name'),'valid':row['valid'],'intent':row['intent_pass'],'ms':row['elapsed_ms']}),flush=True)
 # A natural conversation with preservation, a combined change and a reply-only follow-up.
 own();state=request('POST','/api/labs/llm/reset',{'spec':initial})
 for message in ['Set the anchor at the tip. Use irregular drift and keep the pace at 25.','Soften both turnarounds to 70, and keep everything else unchanged.','Five speed points lower, please. Keep the drift and softness.','What settings are we testing? Explain briefly without changing them.']:
  state=request('POST','/api/labs/llm/chat',{'message':message,'method':'edits','model':model,'revision':state['revision'],'schema_guided':True})
  row=state['turns'][-1];row['evaluation']='conversation';turns.append(row);save();print(json.dumps({'model':model,'conversation':message,'valid':row['valid'],'changed':row['changed']}),flush=True)
 # Compound requests are independent: fluent prose can conceal omitted controls.
 compound=[
  ('Use tip-anchored drift with softer turnarounds at 70. Keep the pace at 25.', {'anchor_percent':100,'variation_mode':'drift','turn_softness_percent':70}),
  ('Keep the center at 50, use drift, and set softness to 55 with a fully steady beat. Preserve speed.', {'anchor_percent':50,'variation_mode':'drift','turn_softness_percent':55,'cadence_hold_percent':100}),
  ('Keep all settings unchanged and just describe them.', {}),
 ]
 for message,expected in compound:
  own();state=request('POST','/api/labs/llm/reset',{'spec':initial})
  state=request('POST','/api/labs/llm/chat',{'message':message,'method':'edits','model':model,'revision':state['revision'],'schema_guided':True})
  row=state['turns'][-1];row['evaluation']='compound-controls';row['expected_controls']=expected;row['expected']=json.dumps(expected,sort_keys=True) if expected else 'No control changes'
  row['intent_pass']=row['valid'] and row['after']['speed_percent']==25 and (all(row['after'].get(key)==value for key,value in expected.items()) if expected else not row['changed'])
  turns.append(row);save();print(json.dumps({'model':model,'compound':message,'valid':row['valid'],'intent':row['intent_pass'],'changed':row['changed']}),flush=True)
 # Two real scheduled turns through this app. This is preview-only: no device configuration or motion.
 state=request('POST','/api/labs/llm/session',{'method':'edits','model':model,'autopilot':True,'live':False,'schema_guided':True,'interval_seconds':5})
 start_count=len(state['turns']); deadline=time.monotonic()+100
 try:
  while time.monotonic()<deadline:
   time.sleep(1); state=request('GET','/api/labs/llm')
   if len(state['turns'])>=start_count+2 or not state['session']['autopilot']:break
 finally:
  request('POST','/api/motion/stop',{})
 for row in state['turns'][start_count:]:row['evaluation']='scheduled-autopilot';turns.append(row);save()
 print(json.dumps({'model':model,'scheduled_turns':len(state['turns'])-start_count,'session_error':state['session'].get('error')}),flush=True)
print(json.dumps({'trials':len(turns),'valid':sum(row['valid'] for row in turns),'intent_pass':sum(row.get('intent_pass',False) for row in turns),'intent_trials':sum('intent_pass' in row for row in turns)}),flush=True)
