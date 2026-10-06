"""Evaluate captured Freestyle sessions: rebuild the executed point stream,
locate planner events on it, and measure smoothness, holds, transitions and
variety. Usage: evaluate-freestyle.py capture.json output_dir [label]"""
import io
import json
import os
import sys
from datetime import datetime

import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

capture_path, out_dir = sys.argv[1], sys.argv[2]
label = sys.argv[3] if len(sys.argv) > 3 else 'freestyle'
os.makedirs(out_dir, exist_ok=True)
capture = json.load(io.open(capture_path, encoding='utf-8'))


def parse_time(text):
    text = text.replace('Z', '+00:00')
    if '.' in text:
        head, rest = text.split('.', 1)
        digits = ''.join(ch for ch in rest if ch.isdigit())
        tz = rest[len(digits):]
        text = head + '.' + digits[:6].ljust(6, '0') + tz
    return datetime.fromisoformat(text).timestamp()


def stream(session):
    play = None
    points = []
    for command in session['commands']:
        if command.get('kind') == 'points_play' and play is None:
            play = command
        add = command.get('points_add')
        if add:
            points.extend((p['time_ms'], p['position_percent']) for p in add['points'])
    points.sort()
    times = np.array([p[0] for p in points], dtype=float) / 1000.0
    positions = np.array([p[1] for p in points], dtype=float)
    keep = np.concatenate(([True], np.diff(times) > 0))
    times, positions = times[keep], positions[keep]
    origin_wall = parse_time(play['issued_at']) if play else None
    start_ms = (play.get('points_play') or {}).get('start_time_ms', 0) / 1000.0 if play else 0.0
    stops = [parse_time(c['issued_at']) - origin_wall + start_ms
             for c in session['commands'] if c.get('kind') == 'stop' and origin_wall is not None]
    return times, positions, origin_wall, start_ms, min(stops, default=float('inf'))


def planner_events(session, origin_wall, start_s):
    events = []
    for row in session['trace_rows']:
        planner = row.get('planner')
        if not planner or planner.get('mode') != 'freestyle':
            continue
        if planner.get('event') not in ('freestyle_start', 'freestyle_segment', 'segment_drift', 'freestyle_continue',
                                        'freestyle_preferences', 'freestyle_limits', 'freestyle_shape', 'freestyle_ending'):
            continue
        at = parse_time(row['timestamp']) - origin_wall + start_s if origin_wall else 0.0
        label = (planner.get('pattern_id') or '').replace('flow-', '')
        if label == 'freestyle_stream':
            label = planner['event'].replace('freestyle_', '')
            note = planner.get('note') or ''
            for part in note.split():
                if part.startswith('phase=') and part != 'phase=steady':
                    label += ' ' + part[6:]
        events.append({'t': at, 'event': planner['event'], 'pattern': label,
                       'speed': planner.get('speed_percent'), 'drift': planner.get('drift_to_percent'),
                       'duration': planner.get('duration_ms')})
    return events


def resample(times, positions, rate=100.0):
    grid = np.arange(times[0], times[-1], 1.0 / rate)
    return grid, np.interp(grid, times, positions)


def strokes(grid, x):
    velocity = np.gradient(x, grid)
    sign = np.sign(velocity)
    sign[sign == 0] = 1
    turns = np.where(np.diff(sign) != 0)[0]
    legs = []
    for a, b in zip(turns[:-1], turns[1:]):
        legs.append((grid[a], grid[b] - grid[a], abs(x[b] - x[a]), x[a], x[b]))
    return velocity, legs


summary = {}
sessions = capture['sessions']
fig_all, axes_all = plt.subplots(len(sessions), 1, figsize=(18, 4.2 * len(sessions)), squeeze=False)
for row_index, session in enumerate(sessions):
    style = session['style']
    times, positions, origin_wall, start_s, stop_s = stream(session)
    events = planner_events(session, origin_wall, start_s)
    full_grid, full_x = resample(times, positions)
    played = full_grid <= stop_s
    grid, x = full_grid[played], full_x[played]
    velocity, legs = strokes(grid, x)
    acceleration = np.gradient(velocity, grid)
    speed = np.abs(velocity)
    # Holds: stretches of at least 300 ms that barely move.
    still = speed < 3.0
    holds, run = [], 0
    for index, flag in enumerate(still):
        if flag:
            run += 1
        elif run:
            if run >= 30:
                holds.append((grid[index - run], run / 100.0))
            run = 0
    segments = [e for e in events if e['event'] != 'segment_drift']
    boundary = []
    steady_acc = np.percentile(np.abs(acceleration), 99)
    for event in segments[1:]:
        window = (grid >= event['t'] - 1.0) & (grid <= event['t'] + 4.0)
        if not window.any():
            continue
        boundary.append({'t': round(event['t'], 2), 'to': event['pattern'], 'speed': event['speed'],
                         'peak_speed': round(float(speed[window].max()), 1),
                         'peak_accel': round(float(np.abs(acceleration[window]).max()), 1),
                         'hold_s': round(sum(d for (s, d) in holds if event['t'] - 1 <= s <= event['t'] + 4), 2)})
    amplitudes = np.array([leg[2] for leg in legs if leg[2] >= 2])
    durations = np.array([leg[1] for leg in legs if leg[2] >= 2])
    summary[style] = {
        'seconds': round(float(grid[-1] - grid[0]), 1),
        'plays': sum(c.get('kind') == 'points_play' for c in session['commands']),
        'stops': sum(c.get('kind') == 'stop' for c in session['commands']),
        'appends': sum(c.get('kind') == 'points_add' for c in session['commands']),
        'retarget_blends': sum(bool((r.get('retarget') or {}).get('bridge_points_inserted'))
                              for r in session['trace_rows']),
        'cancelled_tail_seconds': round(float(full_grid[-1] - grid[-1]), 2),
        'segments': len(segments), 'drifts': sum(1 for e in events if e['event'] == 'segment_drift'),
        'patterns': [e['pattern'] for e in segments],
        'speeds': [e['speed'] for e in segments],
        'strokes': int(len(amplitudes)),
        'stroke_amplitude_p10_p50_p90': [round(float(np.percentile(amplitudes, q)), 1) for q in (10, 50, 90)] if len(amplitudes) else [],
        'half_cycle_s_p10_p50_p90': [round(float(np.percentile(durations, q)), 2) for q in (10, 50, 90)] if len(durations) else [],
        'peak_speed_pct_per_s': round(float(speed.max()), 1),
        'speed_p50_p95': [round(float(np.percentile(speed, q)), 1) for q in (50, 95)],
        'accel_p99': round(float(steady_acc), 1),
        'holds_over_300ms': len(holds), 'hold_seconds_total': round(sum(d for _, d in holds), 2),
        'longest_hold_s': round(max((d for _, d in holds), default=0.0), 2),
        'boundaries': boundary,
    }
    axis = axes_all[row_index][0]
    axis.plot(full_grid - grid[0], full_x, linewidth=0.6, color='#2b6cb0')
    if stop_s < full_grid[-1]:
        axis.axvspan(stop_s - grid[0], full_grid[-1] - grid[0], color='#777777', alpha=0.25,
                     label='Queued points cancelled by Stop')
    for event in events:
        color = '#c53030' if event['event'] != 'segment_drift' else '#dd6b20'
        axis.axvline(event['t'] - grid[0], color=color, linewidth=0.8, linestyle='--' if event['event'] == 'segment_drift' else '-')
        if event['event'] != 'segment_drift':
            axis.text(event['t'] - grid[0] + 0.3, 101, '%s %s%%' % (event['pattern'], event['speed']), fontsize=7, rotation=0, va='bottom')
    for start, duration in holds:
        axis.axvspan(start - grid[0], start - grid[0] + duration, color='#f6ad55', alpha=0.35)
    axis.set_ylim(-3, 112)
    axis.set_xlim(0, full_grid[-1] - grid[0])
    axis.set_title('%s: %s style, %d segments (red), drifts dashed, holds shaded' % (label, style, len(segments)), fontsize=10)
    axis.set_ylabel('position %')

    # Zoomed boundaries with velocity.
    picks = segments[1:5]
    if picks:
        fig, axes = plt.subplots(2, len(picks), figsize=(4.6 * len(picks), 6), squeeze=False)
        for column, event in enumerate(picks):
            window = (grid >= event['t'] - 4) & (grid <= event['t'] + 6)
            axes[0][column].plot(grid[window] - event['t'], x[window], color='#2b6cb0', linewidth=0.9)
            axes[0][column].axvline(0, color='#c53030')
            axes[0][column].set_title('-> %s %s%%' % (event['pattern'], event['speed']), fontsize=9)
            axes[0][column].set_ylim(-3, 103)
            axes[1][column].plot(grid[window] - event['t'], velocity[window], color='#2f855a', linewidth=0.8)
            axes[1][column].axvline(0, color='#c53030')
            axes[1][column].set_ylabel('velocity %/s', fontsize=8)
        fig.suptitle('%s %s: segment boundaries (t=0), position and velocity' % (label, style), fontsize=10)
        fig.tight_layout()
        fig.savefig(os.path.join(out_dir, '%s-%s-boundaries.png' % (label, style)), dpi=110)
        plt.close(fig)

    # Stroke envelope over time: amplitude and half-cycle duration per stroke.
    fig, axes = plt.subplots(2, 1, figsize=(18, 5), sharex=True)
    leg_t = np.array([leg[0] for leg in legs if leg[2] >= 2]) - grid[0]
    if len(leg_t):
        axes[0].scatter(leg_t, amplitudes, s=5, color='#2b6cb0')
        axes[1].scatter(leg_t, durations, s=5, color='#555555')
    for event in segments:
        for axis2 in axes:
            axis2.axvline(event['t'] - grid[0], color='#c53030', linewidth=0.7)
    axes[0].set_ylabel('stroke length %')
    axes[1].set_ylabel('half-cycle s')
    axes[0].set_title('%s %s: per-stroke length and duration (segment changes in red)' % (label, style), fontsize=10)
    fig.tight_layout()
    fig.savefig(os.path.join(out_dir, '%s-%s-strokes.png' % (label, style)), dpi=110)
    plt.close(fig)

fig_all.suptitle('Shared engine commands: estimated playback, not measured device position. Grey tail is cancelled.', fontsize=11)
fig_all.tight_layout()
fig_all.savefig(os.path.join(out_dir, '%s-timelines.png' % label), dpi=110)
plt.close(fig_all)
io.open(os.path.join(out_dir, '%s-summary.json' % label), 'w', encoding='utf-8').write(json.dumps(summary, indent=2))
for style, data in summary.items():
    brief = {k: v for k, v in data.items() if k not in ('boundaries', 'patterns', 'speeds')}
    print(style, json.dumps(brief))
    for b in data['boundaries'][:12]:
        print('   boundary', b)
