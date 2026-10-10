// A 5 × 7 dot font for the dot-display readout, in the manner of the Handy 2's
// own display. Each glyph is seven rows of five columns; "1" is a lit dot.
const GLYPHS: Record<string, readonly string[]> = {
  "0": ["01110", "10001", "10011", "10101", "11001", "10001", "01110"],
  "1": ["00100", "01100", "00100", "00100", "00100", "00100", "01110"],
  "2": ["01110", "10001", "00001", "00010", "00100", "01000", "11111"],
  "3": ["11110", "00001", "00001", "01110", "00001", "00001", "11110"],
  "4": ["00010", "00110", "01010", "10010", "11111", "00010", "00010"],
  "5": ["11111", "10000", "11110", "00001", "00001", "10001", "01110"],
  "6": ["01110", "10000", "10000", "11110", "10001", "10001", "01110"],
  "7": ["11111", "00001", "00010", "00100", "01000", "01000", "01000"],
  "8": ["01110", "10001", "10001", "01110", "10001", "10001", "01110"],
  "9": ["01110", "10001", "10001", "01111", "00001", "00010", "01100"],
  "-": ["00000", "00000", "00000", "11111", "00000", "00000", "00000"],
};

export interface Dot {
  x: number;
  y: number;
  lit: boolean;
}

/** Glyph width in dot columns, and the gap between glyphs. */
const COLUMNS = 5;
const ADVANCE = 6;

/** Width of a text run in pitches, from the first dot centre to the last. */
export function dotTextSpan(text: string): number {
  return text.length ? (text.length - 1) * ADVANCE + COLUMNS - 1 : 0;
}

/**
 * Dots for `text`, horizontally centred on `centerX` with its top row at `top`.
 * Characters without a glyph render as blank cells.
 */
export function dotText(text: string, centerX: number, top: number, pitch: number): Dot[] {
  const left = centerX - (dotTextSpan(text) * pitch) / 2;
  const dots: Dot[] = [];
  Array.from(text).forEach((character, index) => {
    const glyph = GLYPHS[character];
    for (let row = 0; row < 7; row += 1) {
      for (let column = 0; column < COLUMNS; column += 1) {
        dots.push({
          x: left + (index * ADVANCE + column) * pitch,
          y: top + row * pitch,
          lit: glyph?.[row]?.[column] === "1",
        });
      }
    }
  });
  return dots;
}
