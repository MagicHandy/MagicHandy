import { type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { motion } from "motion/react";

const STAGE_SPRING = {
  type: "spring" as const,
  stiffness: 420,
  damping: 36,
  mass: 0.7,
};

type Props = {
  active: boolean;
  recording: boolean;
  fastResponse?: boolean;
  targetPct: number;
  sentPct: number;
  padRef: RefObject<HTMLDivElement>;
  onPointerMove: (clientY: number) => void;
  onPointerLeave: () => void;
  onPadPointerDown: (e: React.PointerEvent<HTMLDivElement>) => void;
  onPadPointerUp: (e: React.PointerEvent<HTMLDivElement>) => void;
  onPadPointerCancel: (e: React.PointerEvent<HTMLDivElement>) => void;
  onPadContextMenu: (e: React.MouseEvent<HTMLDivElement>) => void;
  onPadAuxClick: (e: React.MouseEvent<HTMLDivElement>) => void;
};

const VB_W = 200;
const VB_H = 420;
const TUBE_TOP = 36;
const TUBE_BOTTOM = 384;
const TUBE_H = TUBE_BOTTOM - TUBE_TOP;
const CX = VB_W / 2;
const RX = 72;

const TARGET_SPRING = {
  type: "spring" as const,
  stiffness: 720,
  damping: 42,
  mass: 0.55,
};

const SENT_SPRING = {
  type: "spring" as const,
  stiffness: 160,
  damping: 26,
  mass: 0.85,
};

function clampPct(value: number): number {
  return Math.max(0, Math.min(100, value));
}

function pctToY(pct: number): number {
  return TUBE_BOTTOM - (clampPct(pct) / 100) * TUBE_H;
}

export function MouseControlCylinder({
  active,
  recording,
  fastResponse = false,
  targetPct,
  sentPct,
  padRef,
  onPointerMove,
  onPointerLeave,
  onPadPointerDown,
  onPadPointerUp,
  onPadPointerCancel,
  onPadContextMenu,
  onPadAuxClick,
}: Props) {
  const { t } = useTranslation();
  const target = clampPct(targetPct);
  const sent = clampPct(sentPct);
  const targetY = pctToY(target);
  const sentY = pctToY(sent);
  const roundedTarget = Math.round(target);

  return (
    <motion.div
      className={`mouse-cylinder-widget${active ? " mouse-cylinder-widget--live" : ""}${recording ? " mouse-cylinder-widget--recording" : ""}`}
      initial={false}
      animate={{
        scale: active ? 1 : 0.97,
        y: active ? 0 : 4,
      }}
      transition={{ type: "spring", stiffness: 320, damping: 28 }}
    >
      <div
        ref={padRef}
        className={`mouse-cylinder-pad${active ? " mouse-cylinder-pad--live" : ""}${fastResponse ? " mouse-cylinder-pad--fast" : ""}${recording ? " mouse-cylinder-pad--recording" : ""}`}
        onPointerMove={(e) => active && onPointerMove(e.clientY)}
        onPointerLeave={() => active && onPointerLeave()}
        onPointerEnter={(e) => active && onPointerMove(e.clientY)}
        onPointerDown={onPadPointerDown}
        onPointerUp={onPadPointerUp}
        onPointerCancel={onPadPointerCancel}
        onContextMenu={onPadContextMenu}
        onAuxClick={onPadAuxClick}
        role="application"
        aria-label={t("mouse.cylinder.padAria")}
        aria-disabled={!active}
      >
        {active && (
          <motion.div
            className="mouse-cylinder-ghost-line"
            aria-hidden
            initial={{ opacity: 0 }}
            animate={{
              opacity: 1,
              top: `${100 - target}%`,
            }}
            transition={{
              opacity: { duration: 0.15 },
              top: STAGE_SPRING,
            }}
          />
        )}

        <motion.svg
          className="mouse-cylinder-svg"
          viewBox={`0 0 ${VB_W} ${VB_H}`}
          role="img"
          aria-label={t("mouse.cylinder.positionAria", { value: roundedTarget })}
          initial={false}
          animate={{
            opacity: active ? 1 : 0.82,
          }}
          transition={{ duration: 0.25 }}
        >
          <defs>
            <linearGradient id="mc-tube-body" x1="0%" y1="0%" x2="100%" y2="0%">
              <stop offset="0%" stopColor="#0a0c14" stopOpacity="0.95" />
              <stop offset="18%" stopColor="#3d4668" stopOpacity="0.35" />
              <stop offset="50%" stopColor="#8b9ad4" stopOpacity="0.12" />
              <stop offset="82%" stopColor="#3d4668" stopOpacity="0.35" />
              <stop offset="100%" stopColor="#0a0c14" stopOpacity="0.95" />
            </linearGradient>
            <linearGradient id="mc-cap-top" x1="50%" y1="0%" x2="50%" y2="100%">
              <stop offset="0%" stopColor="#5c6a94" stopOpacity="0.5" />
              <stop offset="100%" stopColor="#12141f" stopOpacity="0.95" />
            </linearGradient>
            <linearGradient id="mc-cap-bot" x1="50%" y1="0%" x2="50%" y2="100%">
              <stop offset="0%" stopColor="#1a1d2a" stopOpacity="0.9" />
              <stop offset="100%" stopColor="#08090f" stopOpacity="1" />
            </linearGradient>
            <radialGradient id="mc-blade-target" cx="50%" cy="50%" r="50%">
              <stop offset="0%" stopColor="#ddd6fe" stopOpacity="0.75" />
              <stop offset="55%" stopColor="#818cf8" stopOpacity="0.35" />
              <stop offset="100%" stopColor="#6366f1" stopOpacity="0" />
            </radialGradient>
            <radialGradient id="mc-blade-sent" cx="50%" cy="50%" r="50%">
              <stop offset="0%" stopColor="#ffffff" stopOpacity="0.45" />
              <stop offset="60%" stopColor="#cbd5e1" stopOpacity="0.15" />
              <stop offset="100%" stopColor="#ffffff" stopOpacity="0" />
            </radialGradient>
            <filter id="mc-glow" x="-50%" y="-50%" width="200%" height="200%">
              <feGaussianBlur stdDeviation="3" result="blur" />
              <feMerge>
                <feMergeNode in="blur" />
                <feMergeNode in="SourceGraphic" />
              </feMerge>
            </filter>
            <clipPath id="mc-tube-clip">
              <rect x={CX - RX} y={TUBE_TOP} width={RX * 2} height={TUBE_H} rx="8" />
            </clipPath>
          </defs>

          <ellipse
            cx={CX}
            cy={TUBE_BOTTOM + 22}
            rx={RX * 0.85}
            ry="10"
            fill="rgba(0,0,0,0.45)"
          />

          <ellipse
            cx={CX}
            cy={TUBE_BOTTOM}
            rx={RX}
            ry="16"
            fill="url(#mc-cap-bot)"
            stroke="rgba(148,163,184,0.25)"
            strokeWidth="1"
          />

          <rect
            x={CX - RX}
            y={TUBE_TOP}
            width={RX * 2}
            height={TUBE_H}
            fill="url(#mc-tube-body)"
            stroke="rgba(148,163,184,0.22)"
            strokeWidth="1"
          />

          <rect
            x={CX - RX + 6}
            y={TUBE_TOP + 4}
            width={RX * 2 - 12}
            height={TUBE_H - 8}
            fill="none"
            stroke="rgba(99,102,241,0.08)"
            strokeWidth="1"
            rx="6"
          />

          <g clipPath="url(#mc-tube-clip)">
            <motion.ellipse
              cx={CX}
              rx={RX - 6}
              ry="9"
              fill="url(#mc-blade-sent)"
              stroke="rgba(255,255,255,0.2)"
              strokeWidth="0.75"
              className="mouse-cylinder-blade-sent"
              initial={false}
              animate={{ cy: sentY }}
              transition={SENT_SPRING}
            />
            <motion.ellipse
              cx={CX}
              rx={RX - 4}
              ry="11"
              fill="url(#mc-blade-target)"
              stroke="rgba(167,139,250,0.65)"
              strokeWidth="1"
              filter="url(#mc-glow)"
              initial={false}
              animate={{
                cy: targetY,
                strokeOpacity: active ? 0.85 : 0.45,
              }}
              transition={{
                cy: TARGET_SPRING,
                strokeOpacity: { duration: 0.2 },
              }}
            />
            <motion.line
              x1={CX - RX + 12}
              x2={CX + RX - 12}
              stroke="rgba(255,255,255,0.35)"
              strokeWidth="0.5"
              strokeLinecap="round"
              initial={false}
              animate={{ y1: targetY, y2: targetY }}
              transition={TARGET_SPRING}
            />
            {active && (
              <motion.rect
                x={CX - RX + 8}
                width={RX * 2 - 16}
                height={TUBE_H}
                fill="url(#mc-blade-target)"
                opacity={0.04}
                initial={false}
                animate={{
                  y: targetY,
                  height: Math.max(8, TUBE_BOTTOM - targetY),
                }}
                transition={TARGET_SPRING}
              />
            )}
          </g>

          <ellipse
            cx={CX}
            cy={TUBE_TOP}
            rx={RX}
            ry="16"
            fill="url(#mc-cap-top)"
            stroke="rgba(148,163,184,0.3)"
            strokeWidth="1"
          />

          <ellipse
            cx={CX}
            cy={TUBE_TOP + 1}
            rx={RX - 8}
            ry="8"
            fill="none"
            stroke="rgba(255,255,255,0.08)"
            strokeWidth="1"
          />

          {[0, 25, 50, 75, 100].map((tick) => {
            const y = pctToY(tick);
            return (
              <g key={tick} opacity="0.45">
                <line
                  x1={CX + RX + 6}
                  x2={CX + RX + 14}
                  y1={y}
                  y2={y}
                  stroke="rgba(148,163,184,0.5)"
                  strokeWidth="1"
                />
                <text
                  x={CX + RX + 18}
                  y={y + 3}
                  fill="rgba(148,163,184,0.55)"
                  fontSize="9"
                  fontFamily="var(--mono, monospace)"
                >
                  {tick}
                </text>
              </g>
            );
          })}
        </motion.svg>
      </div>

      <div className="mouse-cylinder-readout" aria-hidden>
        <motion.span
          key={roundedTarget}
          className="mouse-cylinder-readout-value mono"
          initial={{ opacity: 0.55, y: 6, scale: 0.94 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ type: "spring", stiffness: 480, damping: 32 }}
        >
          {roundedTarget}%
        </motion.span>
      </div>
    </motion.div>
  );
}
