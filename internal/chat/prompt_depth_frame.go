package chat

// depthFrame is the physical reading of the motion slider that every motion
// mode and planner shares. The continuous contracts named the ends only as
// numbers, so a partner voice acting out a scene could send "deeper" toward
// the tip while narrating the opposite. Defining depth once, instead of
// mapping particular phrases, lets the model read any request or scene in the
// frame the engine actually plays.
const depthFrame = `DEPTH FRAME:
Every motion position is a depth along one stroke. 0 is the base, the deepest point; 100 is the tip, the shallowest point; a stroke over the whole length travels between them. Deeper or further down means lower values; shallower or further up means higher values. Depth in any scene you describe is depth along this stroke. Read requests in this frame, set every control that decides where the strokes travel, and describe only the depth they actually reach.`
