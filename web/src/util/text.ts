export function codePointLength(value: string): number {
  return Array.from(value).length;
}

export function limitCodePoints(value: string, limit: number): string {
  const points = Array.from(value);
  return points.length <= limit ? value : points.slice(0, limit).join("");
}
