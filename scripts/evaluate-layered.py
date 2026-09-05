#!/usr/bin/env python3
import json,urllib.request,time,sys
from pathlib import Path
import argparse
parser=argparse.ArgumentParser(description='Evaluate Layered on an isolated app; takes controller ownership and uses preview-only sessions.')
parser.add_argument('--base-url',required=True)
parser.add_argument('--model',action='append',required=True)
parser.add_argument('--output',required=True)
args=parser.parse_args()
BASE=args.base_url.rstrip('/')
HEAD={'Content-Type':'application/json','X-MagicHandy-Client-ID':'layered-evaluation'}
output=Path(args.output);output.parent.mkdir(parents=True,exist_ok=True);turns=[]
def req(method,path,body=None):
 data=None if body is None else json.dumps(body).encode()
 with urllib.request.urlopen(urllib.request.Request(BASE+path,data=data,method=method,headers=HEAD),timeout=180) as r:return json.load(r)
def save(row,case,passed):
 row.update(evaluation=case,intent_pass=passed,expected_recipe=case)
 turns.append(row);output.write_text(json.dumps({'turns':turns},ensure_ascii=False,indent=2),encoding='utf-8')
 print(json.dumps({'model':row['model'],'case':case,'valid':row['valid'],'intent':passed,'changed':row['changed'],'reply':row.get('reply'),'error':row.get('error'),'ms':row['elapsed_ms']}),flush=True)
def axis(s,a):return next((l for l in s.get('layers',[]) if l['axis']==a),{})
def local(s):return 10<=s['range_floor_percent']<=s.get('range_ceiling_percent',90)<=30 and axis(s,'center').get('shape')=='alternate' and axis(s,'center').get('amount_percent')==100 and not axis(s,'range')
def broad_tip(s):return s['anchor_percent']==100 and s['range_floor_percent']<=30 and s.get('range_ceiling_percent',0) in [0,s['max_percent']-s['min_percent']] and not axis(s,'center') and axis(s,'range').get('shape')=='alternate' and axis(s,'range').get('amount_percent')==100
def score_without_seed(s):return {k:v for k,v in s.items() if k!='seed'}
req('POST','/api/controller/takeover',{})
req('POST','/api/motion/stop',{})
for model in args.model:
 state=req('POST','/api/labs/llm/reset',{'method':'layered'})
 for message,case,check in [
  ('alternate between tip and base','short strokes alternate ends',local),
  ('Nah, jerk the base then jerk the tip and alternate','preserve localized alternation',local),
  ('jerk gently','gentler pace with same alternation',lambda s:local(s) and s['speed_percent']<25),
  ('alternate between full strokes and hammering the tip','broad and short tip-anchored strokes',broad_tip),
 ]:
  state=req('POST','/api/labs/llm/chat',{'message':message,'method':'layered','model':model,'revision':state['revision'],'schema_guided':True})
  row=state['turns'][-1];save(row,case,row['valid'] and check(row['after']))
 state=req('POST','/api/labs/llm/session',{'method':'layered','model':model,'autopilot':True,'live':False,'schema_guided':True,'interval_seconds':5})
 revision=state['revision'];count=0;deadline=time.monotonic()+200
 while time.monotonic()<deadline and count<8:
  time.sleep(1);req('GET','/api/controller');state=req('GET','/api/labs/llm')
  if state['revision']>revision:
   new=state['revision']-revision;revision=state['revision']
   for row in state['turns'][-new:]:
    save(row,'scheduled ongoing variation',row['valid'] and broad_tip(row['after']) and row['after']['seed']!=row['before']['seed'] and row['after']['speed_percent']==row['before']['speed_percent']);count+=1
  if not state.get('session',{}).get('autopilot'):break
 req('POST','/api/motion/stop',{})
 state=req('GET','/api/labs/llm')
 for message,case,check in [
  ('What are the current layers doing? Explain briefly without changing anything.','ordinary question holds',lambda row:score_without_seed(row['before'])==score_without_seed(row['after']) and row['before']['seed']==row['after']['seed']),
  ('Keep this exact pattern repeating. No changes from now on.','explicit exact repetition',lambda row:row['before']==row['after']),
 ]:
  state=req('POST','/api/labs/llm/chat',{'message':message,'method':'layered','model':model,'revision':state['revision'],'schema_guided':True})
  row=state['turns'][-1];save(row,case,row['valid'] and check(row))
 state=req('POST','/api/labs/llm/session',{'method':'layered','model':model,'autopilot':True,'live':False,'schema_guided':True,'interval_seconds':5});revision=state['revision'];deadline=time.monotonic()+80
 while time.monotonic()<deadline:
  time.sleep(1);req('GET','/api/controller');state=req('GET','/api/labs/llm')
  if state['revision']>revision:
   row=state['turns'][-1];save(row,'autopilot honors exact repetition',row['valid'] and row['before']==row['after']);break
  if not state.get('session',{}).get('autopilot'):break
 req('POST','/api/motion/stop',{})
 # Wording absent from the prompt examples and independent refinements.
 for message,case,check in [
  ('Use short strokes in the lower region for a while, then the upper region, and keep alternating.','held-out alternating ends',lambda row:local(row['after'])),
  ('Alternate full travel with brief strokes at the upper end.','held-out full and tip',lambda row:broad_tip(row['after'])),
  ('Alternate full travel with brief strokes at the lower end.','held-out full and base',lambda row:row['after']['anchor_percent']==0 and row['after'].get('range_ceiling_percent')==90 and axis(row['after'],'range').get('shape')=='alternate' and not axis(row['after'],'center')),
  ('Keep one endpoint at the tip while reach changes irregularly.','held-out tip anchor',lambda row:row['after']['anchor_percent']==100 and not axis(row['after'],'center') and row['after']['range_floor_percent']<row['after'].get('range_ceiling_percent',90)),
  ('Keep one endpoint at the base while reach changes irregularly.','held-out base anchor',lambda row:row['after']['anchor_percent']==0 and not axis(row['after'],'center')),
  ('Make only the pace layer develop more slowly; keep speed and reach unchanged.','independent pace timing',lambda row:axis(row['after'],'pace').get('period_cycles',0)>22 and row['after']['speed_percent']==25 and axis(row['after'],'center')==axis(row['before'],'center')),
  ('Make strokes exactly 30 wide and let the window move irregularly.','paired width and location',lambda row:row['after']['range_floor_percent']==30 and row['after'].get('range_ceiling_percent')==30 and axis(row['after'],'center').get('shape')=='drift'),
  ("Reduce speed by 3 points and increase the pace layer's period by 4 cycles.",'compound relative refinement',lambda row:row['after']['speed_percent']==22 and axis(row['after'],'pace').get('period_cycles')==26 and axis(row['after'],'center')==axis(row['before'],'center')),
 ]:
  state=req('POST','/api/labs/llm/reset',{'method':'layered'})
  state=req('POST','/api/labs/llm/chat',{'message':message,'method':'layered','model':model,'revision':state['revision'],'schema_guided':True})
  row=state['turns'][-1];save(row,case,row['valid'] and check(row))
print(json.dumps({'total':len(turns),'valid':sum(t['valid'] for t in turns),'intent':sum(t['intent_pass'] for t in turns)}),flush=True)
